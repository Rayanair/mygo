let ws;
let currentRoom = "";
let username = "";
let isDrawing = false;
let currentTool = 'brush';  // par défaut, c'est le pinceau
let ctx;
let currentColor = "#000000";
let currentBrushSize = 5;
let isCurrentUserDrawing = false; // Pour savoir si c'est le tour de l'utilisateur de dessiner
let wordModal; // Modale pour afficher le mot à dessiner
let hasGuessed = false; // Empêcher l'utilisateur de deviner à nouveau après avoir trouvé

function connect() {
    ws = new WebSocket("ws://localhost:8080/ws");

    ws.onmessage = function(event) {
        let message = JSON.parse(event.data);

        switch (message.type) {
            case "room_created":
                currentRoom = message.roomId;
                document.getElementById('roomCode').textContent = currentRoom;
                document.getElementById('game').classList.remove('d-none');
                document.getElementById('drawing-tools').classList.remove('d-none');
                document.getElementById('startButton').classList.remove('d-none'); // Affiche le bouton démarrer pour le créateur
                break;
            case "room_joined":
                currentRoom = message.roomId;
                document.getElementById('roomCode').textContent = currentRoom;
                document.getElementById('game').classList.remove('d-none');
                break;
            case "player_list":
                updatePlayerList(message);
                break;
            case "game_started":
                alert(message.content);
                break;
            case "draw_turn":
                alert(message.user + " doit dessiner !");
                isCurrentUserDrawing = message.user === username;
                break;
            case "word_to_draw":
                if (message.user === username) {
                    showWordModal(message.content); // Afficher le mot dans la modale pour le dessinateur
                }
                break;
            case "chat":
                updateChatBox(message);
                break;
            case "draw":
                if (!isCurrentUserDrawing) {
                    drawOnCanvas(message.content);
                }
                break;
            case "game_won":
                alert(message.content);
                break;
        }
    };
}

function updatePlayerList(message) {
    const playerList = document.getElementById('playerList');
    playerList.innerHTML = '';
    const players = JSON.parse(message.content);
    players.forEach(player => {
        const li = document.createElement('li');
        li.className = 'list-group-item';
        li.textContent = `${player.nickname} (${player.points} pts)`;
        if (player.isCreator) {
            const badge = document.createElement('span');
            badge.className = 'badge bg-primary ms-2';
            badge.textContent = 'Propriétaire';
            li.appendChild(badge);
        }
        playerList.appendChild(li);
    });
}

function updateChatBox(message) {
    const chatBox = document.getElementById("chatBox");
    if (message.user === "System") {
        chatBox.value += `*** ${message.content} ***\n`;
    } else {
        chatBox.value += message.user + ": " + message.content + "\n";
    }
    chatBox.scrollTop = chatBox.scrollHeight; // Pour que le chat descende automatiquement
}

function createRoom() {
    username = document.getElementById("username").value;
    if (username === "") {
        alert("Veuillez entrer un pseudo");
        return;
    }

    const msg = {
        type: "create_room",
        user: username
    };
    ws.send(JSON.stringify(msg));
}

function joinRoom() {
    username = document.getElementById("username").value;
    currentRoom = document.getElementById("roomId").value;
    if (username === "" || currentRoom === "") {
        alert("Veuillez entrer un pseudo et un ID de salon valide");
        return;
    }

    const msg = {
        type: "join_room",
        user: username,
        roomId: currentRoom
    };
    ws.send(JSON.stringify(msg));
}

function sendMessage() {
    if (hasGuessed) {
        return; // Empêche d'envoyer un message après avoir deviné correctement
    }

    const chatInput = document.getElementById("chatInput").value;
    const msg = {
        type: "chat",
        content: chatInput,
        user: username,
        roomId: currentRoom
    };
    ws.send(JSON.stringify(msg));
    document.getElementById("chatInput").value = "";
}

function startGame() {
    const msg = {
        type: "start_game",
        user: username,
        roomId: currentRoom
    };
    ws.send(JSON.stringify(msg));
}

// Envoie les coordonnées de dessin à tous les joueurs
function sendDrawingData(x, y, isNewStroke) {
    const msg = {
        type: "draw",
        content: JSON.stringify({
            x: x,
            y: y,
            color: currentColor,
            brushSize: currentBrushSize,
            isNewStroke: isNewStroke // Transmet si c'est un nouveau trait
        }),
        user: username,
        roomId: currentRoom
    };
    ws.send(JSON.stringify(msg));
}

// Outils de dessin
function activateBrush() {
    currentTool = 'brush';
}

function activateEraser() {
    currentTool = 'eraser';
}

function setColor(newColor) {
    currentColor = newColor;
}

function setBrushSize(newSize) {
    currentBrushSize = newSize;
}

// Initialisation du dessin
function initDrawing() {
    const canvas = document.getElementById('canvas');
    ctx = canvas.getContext('2d');

    canvas.addEventListener('mousedown', function(e) {
        if (isCurrentUserDrawing) {
            isDrawing = true;
            ctx.beginPath(); // Démarre un nouveau chemin
            ctx.moveTo(e.offsetX, e.offsetY); // Déplace le curseur au point où l'utilisateur a cliqué
            sendDrawingData(e.offsetX, e.offsetY, true); // isNewStroke est true
        }
    });

    canvas.addEventListener('mousemove', function(e) {
        if (isDrawing && isCurrentUserDrawing) {
            ctx.lineWidth = currentBrushSize;
            ctx.lineCap = 'round';

            if (currentTool === 'brush') {
                ctx.strokeStyle = currentColor;
            } else if (currentTool === 'eraser') {
                ctx.strokeStyle = '#FFFFFF'; // Gomme (blanc)
            }

            ctx.lineTo(e.offsetX, e.offsetY); // Trace vers le nouveau point
            ctx.stroke();
            sendDrawingData(e.offsetX, e.offsetY, false); // isNewStroke est false
        }
    });

    canvas.addEventListener('mouseup', function() {
        if (isCurrentUserDrawing) {
            isDrawing = false;
            ctx.closePath(); // Ferme le chemin après avoir terminé de dessiner
        }
    });

    // Ajout d'un événement pour le `mouseout` pour arrêter de dessiner si la souris sort du canevas
    canvas.addEventListener('mouseout', function() {
        isDrawing = false;
        ctx.closePath();
    });
}

// Afficher le dessin sur les autres clients
function drawOnCanvas(data) {
    const parsedData = JSON.parse(data);

    if (parsedData.isNewStroke) {
        ctx.beginPath(); // Commence un nouveau chemin si c'est un nouveau trait
        ctx.moveTo(parsedData.x, parsedData.y);
    } else {
        ctx.lineWidth = parsedData.brushSize;
        ctx.lineCap = 'round';
        ctx.strokeStyle = parsedData.color;
        ctx.lineTo(parsedData.x, parsedData.y);
        ctx.stroke(); // Trace uniquement entre deux points
    }
}

// Affiche le mot à dessiner dans une modale
function showWordModal(word) {
    const wordToDrawElement = document.getElementById('wordToDraw');
    wordToDrawElement.textContent = word;

    const wordModalElement = new bootstrap.Modal(document.getElementById('wordModal'), {
        keyboard: false
    });
    wordModalElement.show();
}

document.getElementById('colorPicker').addEventListener('input', function() {
    setColor(this.value);
});

document.getElementById('brushSize').addEventListener('input', function() {
    setBrushSize(this.value);
});

connect();
initDrawing();
