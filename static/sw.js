self.addEventListener('push', function(event) {
    const message = event.data ? event.data.text() : 'New interaction';
    
    const options = {
        body: message,
        icon: 'icon.png', // Placeholder
        badge: 'icon.png'
    };

    event.waitUntil(
        self.registration.showNotification('Push', options)
    );
});
