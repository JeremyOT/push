const messagesContainer = document.getElementById('messages');
let lastTimestamp = null;

function renderMessage(msg) {
    const msgDiv = document.createElement('div');
    msgDiv.classList.add('message', 'received');
    msgDiv.textContent = msg.message;

    const timeDiv = document.createElement('div');
    timeDiv.classList.add('timestamp');
    const date = new Date(msg.timestamp);
    timeDiv.textContent = date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });

    const wrapper = document.createElement('div');
    wrapper.style.display = 'flex';
    wrapper.style.flexDirection = 'column';
    wrapper.appendChild(msgDiv);
    wrapper.appendChild(timeDiv);

    messagesContainer.appendChild(wrapper);
    messagesContainer.scrollTop = messagesContainer.scrollHeight;
}

async function fetchMessages() {
    try {
        const response = await fetch('/interactions');
        if (!response.ok) throw new Error('Failed to fetch');
        const messages = await response.json();
        
        // Clear current messages to avoid duplicates for now, 
        // or strictly append new ones if we track IDs.
        // Simple approach: clear and redraw (inefficient but safe)
        // Better approach: track last ID.
        
        messagesContainer.innerHTML = '';
        messages.forEach(renderMessage);
        
    } catch (error) {
        console.error('Error fetching messages:', error);
    }
}

// Initial fetch
fetchMessages();

// Poll every 3 seconds
setInterval(fetchMessages, 3000);
