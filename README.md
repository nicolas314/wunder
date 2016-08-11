# wunder
Display weather for the city associated to the requesting IP address

== How to compile and run

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
learning exercise. Use it as a replacement for countless weather apps and
web pages that are either saturated with ads or have too much information.

== Things you should know

If your server is directly internet-facing, the requesting IP address is
run through an online service in charge of geolocating it. If the
requesting IP address is 127.0.0.1, wunder will assume that you are running
an nginx reverse proxy in front of it and obtain the requesting IP by
reading the X-Real-IP header. This should be adapted to your reverse proxy
if you use another one.

The geolocating service is ip-api.com for which you do not need an API key
as long as the number of requests remains reasonable. Wunderground also
offers and IP-based geolocation service but I found it to be less reliable.

The weather page template is located in pages/forecast.html. It can be
easily themed if desired.


