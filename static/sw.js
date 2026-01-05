self.addEventListener('push', function(event) {
    let title = 'Push';
    let options = {
        icon: '/icon.png',
        badge: '/icon.png',
        data: {}
    };

    if (event.data) {
        try {
            const data = event.data.json();
            title = data.title || 'Push';
            options.body = data.message || data.body || '';
            if (data.link) {
                options.data.url = data.link;
            }
        } catch (e) {
            options.body = event.data.text();
        }
    }

    event.waitUntil(
        self.registration.showNotification(title, options)
    );
});

self.addEventListener('notificationclick', function(event) {
    event.notification.close();

    if (event.notification.data && event.notification.data.url) {
        event.waitUntil(
            clients.openWindow(event.notification.data.url)
        );
    }
});
