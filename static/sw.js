self.addEventListener('push', function(event) {
    let title = 'Push';
    let options = {
        icon: '/icon.png',
        badge: '/icon.png'
    };

    if (event.data) {
        try {
            const data = event.data.json();
            title = data.title || 'Push';
            options.body = data.message || data.body || '';
        } catch (e) {
            options.body = event.data.text();
        }
    }

    event.waitUntil(
        self.registration.showNotification(title, options)
    );
});
