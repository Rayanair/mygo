package main

import (
	"encoding/json"
	"golang.org/x/net/websocket"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

// Structure pour un message WebSocket
type Message struct {
	Type    string `json:"type"`
	Content string `json:"content"`
	User    string `json:"user"`
	RoomID  string `json:"roomId,omitempty"`
}

// Structure pour un joueur
type Player struct {
	conn     *websocket.Conn
	nickname string
	points   int
	hasGuessed bool // Indique si le joueur a déjà deviné le mot
}

// Structure pour un salon (room)
type Room struct {
	players    map[*Player]bool
	mu         sync.Mutex
	broadcast  chan Message
	register   chan *Player
	unregister chan *Player
	creator    *Player // Créateur du salon
	drawing    bool    // Indique si une partie est en cours
	currentWord string // Le mot actuel à deviner
}

// Gestionnaire de salons
var rooms = make(map[string]*Room)
var wordList = []string{"chat", "maison", "voiture", "ordinateur", "arbre", "soleil", "montagne", "pomme"}

// Création d'une nouvelle room
func newRoom(creator *Player) *Room {
	room := &Room{
		players:    make(map[*Player]bool),
		broadcast:  make(chan Message),
		register:   make(chan *Player),
		unregister: make(chan *Player),
		creator:    creator,
		drawing:    false,
	}

	// Ajouter le créateur du salon à la liste des joueurs
	room.players[creator] = true

	return room
}

// Fonction pour générer un ID aléatoire pour un salon
func generateRoomID() string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	rand.Seed(time.Now().UnixNano())
	b := make([]byte, 6)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// Sélectionne un mot aléatoire pour le dessinateur
func selectRandomWord() string {
	rand.Seed(time.Now().UnixNano())
	return wordList[rand.Intn(len(wordList))]
}

// Fonction pour envoyer la liste des joueurs à tous les joueurs dans la room
func sendPlayerList(room *Room) {
	playerList := []struct {
		Nickname string `json:"nickname"`
		Points   int    `json:"points"`
		IsCreator bool  `json:"isCreator"`
	}{}
	for player := range room.players {
		playerList = append(playerList, struct {
			Nickname string `json:"nickname"`
			Points   int    `json:"points"`
			IsCreator bool  `json:"isCreator"`
		}{
			Nickname: player.nickname,
			Points:   player.points,
			IsCreator: player == room.creator,
		})
	}

	message := Message{
		Type:    "player_list",
		Content: string(playerListToJSON(playerList)), // Envoi de la liste des joueurs en JSON
	}

	for player := range room.players {
		websocket.JSON.Send(player.conn, message)
	}
}

// Helper pour convertir la liste des joueurs en JSON
func playerListToJSON(players interface{}) []byte {
	jsonData, err := json.Marshal(players)
	if err != nil {
		log.Println("Erreur lors de la conversion de la liste des joueurs en JSON:", err)
	}
	return jsonData
}

// Fonction pour gérer un salon (room)
func (room *Room) run() {
	for {
		select {
		case player := <-room.register:
			room.mu.Lock()
			room.players[player] = true
			room.mu.Unlock()
			// Envoyer la liste des joueurs
			sendPlayerList(room)

		case player := <-room.unregister:
			room.mu.Lock()
			if _, ok := room.players[player]; ok {
				delete(room.players, player)
				player.conn.Close()
			}
			room.mu.Unlock()
			// Mettre à jour la liste des joueurs
			sendPlayerList(room)

		case message := <-room.broadcast:
			room.mu.Lock()
			for player := range room.players {
				err := websocket.JSON.Send(player.conn, message)
				if err != nil {
					log.Println("Erreur lors de l'envoi du message:", err)
				}
			}
			room.mu.Unlock()
		}
	}
}

// Gérer les connexions des joueurs à un salon
func handleConnections(ws *websocket.Conn) {
	var msg Message
	var player *Player

	for {
		err := websocket.JSON.Receive(ws, &msg)
		if err != nil {
			log.Println("Erreur lors de la réception du message:", err)
			if player != nil && msg.RoomID != "" {
				room := rooms[msg.RoomID]
				if room != nil {
					room.unregister <- player
				}
			}
			break
		}

		switch msg.Type {
		case "create_room":
			player = &Player{conn: ws, nickname: msg.User, points: 0, hasGuessed: false}
			roomID := generateRoomID()
			room := newRoom(player)
			rooms[roomID] = room
			go room.run()

			// Envoyer l'ID du salon au créateur
			ws.Write([]byte(`{"type": "room_created", "roomId": "` + roomID + `"}`))

			// Envoyer la liste des joueurs initiale avec le créateur
			sendPlayerList(room)

		case "join_room":
			player = &Player{conn: ws, nickname: msg.User, points: 0, hasGuessed: false}
			room := rooms[msg.RoomID]
			if room != nil {
				room.register <- player
				// Confirmer que le joueur a rejoint le salon
				ws.Write([]byte(`{"type": "room_joined", "roomId": "` + msg.RoomID + `"}`))
			}

		case "start_game":
			room := rooms[msg.RoomID]
			if room != nil && room.creator == player {
				room.drawing = true
				room.broadcast <- Message{Type: "game_started", Content: "La partie commence !"}

				// Sélectionner aléatoirement un dessinateur
				var selectedPlayer *Player
				for p := range room.players {
					selectedPlayer = p
					break
				}

				// Sélectionner un mot à faire deviner
				room.currentWord = selectRandomWord()
				room.broadcast <- Message{Type: "draw_turn", User: selectedPlayer.nickname}

				// Envoyer le mot au dessinateur uniquement
				websocket.JSON.Send(selectedPlayer.conn, Message{Type: "word_to_draw", Content: room.currentWord, User: selectedPlayer.nickname})
			}

		case "chat":
			room := rooms[msg.RoomID]
			if room != nil {
				// Si un joueur a déjà deviné le mot, il ne peut plus envoyer de messages
				if player.hasGuessed {
					continue
				}

				// Vérifier si le message est correct
				if msg.Content == room.currentWord && !player.hasGuessed {
					player.points += 100
					player.hasGuessed = true
					room.broadcast <- Message{
						Type:    "chat",
						Content: player.nickname + " a deviné le mot !",
						User:    "System",
					}

					// Vérifier si un joueur a gagné
					if player.points >= 1000 {
						room.broadcast <- Message{
							Type:    "game_won",
							Content: player.nickname + " a gagné avec 1000 points !",
							User:    "System",
						}
					}

					// Mettre à jour la liste des joueurs avec les scores
					sendPlayerList(room)
				} else {
					// Si le joueur devine incorrectement, on envoie un message normal
					room.broadcast <- msg
				}
			}

		// Nouvelle gestion : Diffusion du dessin
		case "draw":
			room := rooms[msg.RoomID]
			if room != nil {
				// Envoyer le dessin à tous les joueurs de la room
				room.broadcast <- msg
			}
		}
	}
}

func main() {
	http.Handle("/ws", websocket.Handler(handleConnections))

	// Serve static files (for frontend)
	fs := http.FileServer(http.Dir("./frontend"))
	http.Handle("/", fs)

	log.Println("Serveur démarré sur :8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal("Échec du démarrage du serveur:", err)
	}
}
