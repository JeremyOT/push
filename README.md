Push
====

Push is a lightweight one way notification system written in Go that serves a webpage over http, displaying a local database of interactions/messages as a one way chat in an imessage style interface.

It uses the Push API (https://developer.mozilla.org/en-US/docs/Web/API/Push_API) to notify consumers of new interactions when offline, and dynamically updates the chat view as new interactions are posted.

Usage
-----

Run the server as follows:

./push --address=BIND_ADDRESS --port=PORT --database=DATABASE

BIND_ADDRESS defaults to 127.0.0.1
PORT defaults to 8089
DATABASE defaults to "./push.sqlite"

Post new interactions by sending JSON POST requests to http://BIND_ADDRESS:PORT/interactions with the body

{"message": MESSAGE}

Where MESSAGE is the interaction text.

View messages by visiting http://BIND_ADDRESS:PORT

Implementation
--------------

The implementation is a single binary written in Go with embedded html/javascript/css using Golang's embed package. Interactions are stored in a local sqlite database.
