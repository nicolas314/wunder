# wunder
Display weather for the city associated to the requesting IP address

Compile the source with:

    go build wunder.go

You need to obtain an API key from wunderground.com before the server can
be useful. Check their docs: create an account, obtain an API key. Expose
it in your environment by setting it like:

    export WUKey=1234567890abcdef

Start the program by optionally providing a listening address, e.g.

    ./wunder :8080
    ./wunder 127.0.0.1:4242

Place this server on an internet-facing interface and it will respond to
incoming requests by geolocating the incoming IP address, obtaining the
weather forecast for that city, and displaying it. The results should
hopefully be readable on desktop and mobile.

This project started as an exercise to learn how to parse incoming data
from various public APIs, and ended up being a lot more useful than a
learning exercise.
