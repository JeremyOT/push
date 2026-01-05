const messagesContainer = document.getElementById('messages');
let newestId = 0;
let oldestId = 0;
let isLoadingHistory = false;
let initialLoadComplete = false;

function createMessageElement(msg) {
    const msgDiv = document.createElement('div');
    msgDiv.classList.add('message', 'received');
    
    if (msg.link) {
        const link = document.createElement('a');
        link.href = msg.link;
        link.textContent = msg.message;
        link.target = '_blank';
        link.style.color = 'inherit';
        link.style.textDecoration = 'underline';
        msgDiv.appendChild(link);
    } else {
        msgDiv.textContent = msg.message;
    }

    const timeDiv = document.createElement('div');
    timeDiv.classList.add('timestamp');
    const date = new Date(msg.timestamp);
    timeDiv.textContent = date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });

    const wrapper = document.createElement('div');
    wrapper.style.display = 'flex';
    wrapper.style.flexDirection = 'column';
    wrapper.dataset.id = msg.id;
    wrapper.appendChild(msgDiv);
    wrapper.appendChild(timeDiv);
    
    return wrapper;
}

async function fetchMessages(type = 'initial') {
    let url = '/interactions';
    if (type === 'poll') {
        if (!initialLoadComplete) return;
        url += `?after=${newestId}`;
    } else if (type === 'history') {
        url += `?before=${oldestId}`;
    }

    try {
        const response = await fetch(url);
        if (!response.ok) throw new Error('Failed to fetch');
        const messages = await response.json();

        if (messages.length === 0) return;

        if (type === 'initial') {
            messagesContainer.innerHTML = '';
            messages.forEach(msg => {
                messagesContainer.appendChild(createMessageElement(msg));
            });
            if (messages.length > 0) {
                newestId = messages[messages.length - 1].id;
                oldestId = messages[0].id;
            }
            scrollToBottom();
            initialLoadComplete = true;

        } else if (type === 'poll') {
            const wasAtBottom = isAtBottom();
            messages.forEach(msg => {
                messagesContainer.appendChild(createMessageElement(msg));
            });
            if (messages.length > 0) {
                newestId = messages[messages.length - 1].id;
                if (wasAtBottom) scrollToBottom();
            }

        } else if (type === 'history') {
            const previousHeight = messagesContainer.scrollHeight;
            // Prepend in reverse order of the array (which is ASC) so they appear correctly?
            // No, the array is ASC (oldest first). We prepend them one by one.
            // Wait, if we prepend index 0, then index 1 before it? No.
            // We need to prepend the whole batch. 
            // messages: [100, 101, 102] -> prepend 102, then 101? No.
            // We want [100, 101, 102] [Existing 103...]
            
            // Create a fragment
            const fragment = document.createDocumentFragment();
            messages.forEach(msg => {
                fragment.appendChild(createMessageElement(msg));
            });
            messagesContainer.insertBefore(fragment, messagesContainer.firstChild);

            if (messages.length > 0) {
                oldestId = messages[0].id;
            }
            
            // Restore scroll position
            messagesContainer.scrollTop = messagesContainer.scrollHeight - previousHeight;
            isLoadingHistory = false;
        }
        
    } catch (error) {
        console.error('Error fetching messages:', error);
        if (type === 'history') isLoadingHistory = false;
    }
}

function scrollToBottom() {
    messagesContainer.scrollTop = messagesContainer.scrollHeight;
}

function isAtBottom() {
    return messagesContainer.scrollHeight - messagesContainer.scrollTop <= messagesContainer.clientHeight + 50;
}

messagesContainer.addEventListener('scroll', () => {
    if (messagesContainer.scrollTop === 0 && !isLoadingHistory && initialLoadComplete && oldestId > 1) {
        isLoadingHistory = true;
        fetchMessages('history');
    }
});

function urlBase64ToUint8Array(base64String) {
    const padding = '='.repeat((4 - base64String.length % 4) % 4);
    const base64 = (base64String + padding)
        .replace(/\-/g, '+')
        .replace(/\_/g, '/');

    const rawData = window.atob(base64);
    const outputArray = new Uint8Array(rawData.length);

    for (let i = 0; i < rawData.length; ++i) {
        outputArray[i] = rawData.charCodeAt(i);
    }
    return outputArray;
}

const subscribeBtn = document.getElementById('subscribe-btn');

async function checkSubscription() {
    if ('serviceWorker' in navigator && 'PushManager' in window) {
        const registration = await navigator.serviceWorker.ready;
        const subscription = await registration.pushManager.getSubscription();
        if (subscription) {
            subscribeBtn.style.display = 'none';
        } else {
            subscribeBtn.style.display = 'block';
        }
    }
}

async function subscribeToPush() {
    if ('serviceWorker' in navigator && 'PushManager' in window) {
        try {
            const registration = await navigator.serviceWorker.ready;
            
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
            subscribeBtn.style.display = 'none';

        } catch (error) {
            console.error('Service Worker/Push Error:', error);
        }
    }
}

if ('serviceWorker' in navigator) {
    navigator.serviceWorker.register('/sw.js').then(() => {
        console.log('Service Worker registered');
        checkSubscription();
    });
}

subscribeBtn.addEventListener('click', subscribeToPush);

// Initial fetch
fetchMessages('initial');
// Poll every 3 seconds
setInterval(() => fetchMessages('poll'), 3000);
