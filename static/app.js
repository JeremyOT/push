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
        
        messagesContainer.innerHTML = '';
        messages.forEach(renderMessage);
        
    } catch (error) {
        console.error('Error fetching messages:', error);
    }
}

function urlBase64ToUint8Array(base64String) {
    const padding = '='.repeat((4 - base64String.length % 4) % 4);
    const base64 = (base64String + padding)
        .replace(/\-/g, '+')
        .replace(/_/g, '/');

    const rawData = window.atob(base64);
    const outputArray = new Uint8Array(rawData.length);

    for (let i = 0; i < rawData.length; ++i) {
        outputArray[i] = rawData.charCodeAt(i);
    }
    return outputArray;
}

async function registerServiceWorker() {
    if ('serviceWorker' in navigator && 'PushManager' in window) {
        try {
            const registration = await navigator.serviceWorker.register('/sw.js');
            console.log('Service Worker registered');

            const permission = await Notification.requestPermission();
            if (permission !== 'granted') {
                console.log('Notification permission not granted');
                return;
            }

            const response = await fetch('/vapid-public-key');
            const data = await response.json();
            const vapidPublicKey = data.publicKey;
            const convertedVapidKey = urlBase64ToUint8Array(vapidPublicKey);

            let subscription = await registration.pushManager.getSubscription();
            if (!subscription) {
                subscription = await registration.pushManager.subscribe({
                    userVisibleOnly: true,
                    applicationServerKey: convertedVapidKey
                });
            }

            await fetch('/subscribe', {
                method: 'POST',
                body: JSON.stringify(subscription),
                headers: {
                    'Content-Type': 'application/json'
                }
            });
            console.log('Subscribed to push notifications');

        } catch (error) {
            console.error('Service Worker/Push Error:', error);
        }
    }
}

// Initial fetch
fetchMessages();
registerServiceWorker();

// Poll every 3 seconds
setInterval(fetchMessages, 3000);
