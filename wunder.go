//
// Get weather forecast for any IP
// Weather data from wunderground.com
// Results are served by an HTTP server
//
package main

import (
    "encoding/json"
    "fmt"
    "html/template"
    "io"
    "io/ioutil"
    "log"
    "net/http"
    "net/url"
    "os"
    "path"
    "strconv"
    "strings"
    "time"
)

const (
    WUResp          = "resp/"
    Addr            = ":8001"
)

var (
    APIKey  string
)

// To absorb data from ip-api.com
type GeoIP struct {
    // Country     string  `json:"country"`
    CountryCode string  `json:"countryCode"`
    City        string  `json:"city"`
    Lat         float64 `json:"lat"`
    Lon         float64 `json:"lon"`
}

// To absorb data from Google maps
type GooglePos struct {
    Results []struct {
        Geometry struct {
            Location struct {
                Lat float64 `json:"lat"`
                Lon float64 `json:"lng"`
            } `json:"location"`
        } `json:"geometry"`
    } `json:"results"`
}

// Geoloc an IP address, return city + country
func Geolocalize(addr string) (geo GeoIP, err error) {
    resp, err := http.Get("http://ip-api.com/json/" + addr)
    if err != nil {
        fmt.Println(err)
        return
    }
    defer resp.Body.Close()

    err = json.NewDecoder(resp.Body).Decode(&geo)
    if err != nil {
        fmt.Println(err)
        return
    }
    return geo, nil
}

// Geoloc with /country/city
func Position(country, city string) (lat, lon float64, err error) {
    resp, err := http.Get("http://maps.googleapis.com/maps/api/geocode/"+
                          "json?address="+city+","+country)
    if err != nil {
        fmt.Println(err)
        return
    }
    body, err := ioutil.ReadAll(resp.Body)
    resp.Body.Close()
    if err!=nil {
        fmt.Println(err)
        return
    }

    var gp GooglePos
    err = json.Unmarshal(body, &gp)
    if err!=nil {
        fmt.Println(err)
        return
    }
    return gp.Results[0].Geometry.Location.Lat, gp.Results[0].Geometry.Location.Lon, nil
}

// Obtain an icon if not already in cache
// Return absolute (local) path where icon has been downloaded
func CacheIcon(icon string) string {
    // Extract base name
    u, err := url.Parse(icon)
    if err != nil {
        return "/static/empty.png"
    }
    // Download icon
    resp, err := http.Get(icon)
    if err != nil {
        // Cannot download icon: re-direct to original source
        return icon
    }
    body, err := ioutil.ReadAll(resp.Body)
    resp.Body.Close()
    // Save locally
    localname := path.Base(u.Path)
    f, _ := os.Create("static/"+localname)
    f.Write(body)
    f.Close()
    return "/static/" + localname
}

// Absorb data from wunderground
type CurrentConditions struct {
    Cur struct {
        Temp        float64     `json:"temp_c"`
        FeelsLike   string      `json:"feelslike_c"`
        Humidity    string      `json:"relative_humidity"`
        Wind        float64     `json:"wind_kph"`
        Icon        string      `json:"icon_url"`
        LastUpd     string      `json:"observation_time"`
        Weather     string      `json:"weather"`
        ObURL       string      `json:"ob_url"`
        Location struct {
            Country     string  `json:"country"`
            Lat         string  `json:"latitude"`
            Lon         string  `json:"longitude"`
            Alt         string  `json:"elevation"`
            Name        string  `json:"full"`
        } `json:"display_location"`
    } `json:"current_observation"`
    Forecast struct {
        TxtForecast struct {
            Date        string  `json:"date"`
            Day []struct {
                Icon    string  `json:"icon_url"`
                Title   string  `json:"title"`
                Text    string  `json:"fcttext_metric"`
            } `json:"forecastday"`
        } `json:"txt_forecast"`
    } `json:"forecast"`
}

// Get current conditions for Lat, Lon
func GetCurrentByPos(lat, lon float64) (cw CurrentConditions, err error) {
    filename := WUResp+fmt.Sprintf("%g,%g", lat, lon)
    sta, err := os.Stat(filename)
    if err==nil && time.Since(sta.ModTime()) < time.Duration(60*time.Minute) {
        // File exists and is recent: load and return
        content, err := ioutil.ReadFile(filename)
        if err != nil {
            log.Println(err)
            return cw, err
        }
        err = json.Unmarshal(content, &cw)
        if err!=nil {
            log.Println(err)
            return cw, err
        }
        return cw, nil
    }
    // Try geo-localizing incoming address
    var url string
    // Provide geoloc to wunderground
    log.Println("getting current conditions for", lat, lon)
    url ="http://api.wunderground.com/api/"+APIKey+
         "/conditions/forecast/lang:EN/q/"+ fmt.Sprintf("%.2f,%.2f.json", lat, lon)
    resp, err := http.Get(url)
    if err!=nil {
        log.Println(err)
        return cw, err
    }
    defer resp.Body.Close()
    content, err := ioutil.ReadAll(resp.Body)
    if err!=nil {
        log.Println(err)
        return cw, err
    }
    // Validate current weather data
    err = json.Unmarshal(content, &cw)
    if err!=nil {
        log.Println(err)
        return cw, err
    }
    val, _ := strconv.ParseFloat(cw.Cur.Location.Lat, 64)
    cw.Cur.Location.Lat = fmt.Sprintf("%.2f", val)
    val, _  = strconv.ParseFloat(cw.Cur.Location.Lon, 64)
    cw.Cur.Location.Lon = fmt.Sprintf("%.2f", val)
    val, _  = strconv.ParseFloat(cw.Cur.Location.Alt, 64)
    cw.Cur.Location.Alt = fmt.Sprintf("%.2f", val)
    // Replace icons
    cw.Cur.Icon = CacheIcon(cw.Cur.Icon)
    for i:=0 ; i<len(cw.Forecast.TxtForecast.Day) ; i++ {
        cw.Forecast.TxtForecast.Day[i].Icon = CacheIcon(cw.Forecast.TxtForecast.Day[i].Icon)
    }
    // Write data to local file
    f,_ := os.Create(filename)
    out,_ := json.Marshal(cw)
    f.Write(out)
    f.Close()
    return cw, nil
}

// Get current conditions for requesting IP address
func GetCurrentByIP(requester string) (cw CurrentConditions, err error) {
    filename := WUResp+requester
    sta, err := os.Stat(filename)
    if err==nil && time.Since(sta.ModTime()) < time.Duration(60*time.Minute) {
        // File exists and is recent: load and return
        content, err := ioutil.ReadFile(filename)
        if err != nil {
            log.Println(err)
            return cw, err
        }
        err = json.Unmarshal(content, &cw)
        if err!=nil {
            log.Println(err)
            return cw, err
        }
        return cw, nil
    }
    // Try geo-localizing incoming address
    geo, err := Geolocalize(requester)
    var url string
    if err!=nil {
        // Use geoloc from wunderground
        log.Println("getting current conditions (autoip) for", requester)
        url ="http://api.wunderground.com/api/"+APIKey+
             "/conditions/forecast/lang:EN/q/"+
             "autoip.json?geo_ip="+requester
    } else {
        // Provide geoloc to wunderground
        log.Println("getting current conditions (ip-api) for", requester)
        url ="http://api.wunderground.com/api/"+APIKey+
             "/conditions/forecast/lang:EN/q/"+
             fmt.Sprintf("%.2f,%.2f.json", geo.Lat, geo.Lon)
    }
    resp, err := http.Get(url)
    if err!=nil {
        log.Println(err)
        return cw, err
    }
    defer resp.Body.Close()
    content, err := ioutil.ReadAll(resp.Body)
    if err!=nil {
        log.Println(err)
        return cw, err
    }
    // Validate current weather data
    err = json.Unmarshal(content, &cw)
    if err!=nil {
        log.Println(err)
        return cw, err
    }
    val, _ := strconv.ParseFloat(cw.Cur.Location.Lat, 64)
    cw.Cur.Location.Lat = fmt.Sprintf("%.2f", val)
    val, _  = strconv.ParseFloat(cw.Cur.Location.Lon, 64)
    cw.Cur.Location.Lon = fmt.Sprintf("%.2f", val)
    val, _  = strconv.ParseFloat(cw.Cur.Location.Alt, 64)
    cw.Cur.Location.Alt = fmt.Sprintf("%.2f", val)
    // Replace icons
    cw.Cur.Icon = CacheIcon(cw.Cur.Icon)
    for i:=0 ; i<len(cw.Forecast.TxtForecast.Day) ; i++ {
        cw.Forecast.TxtForecast.Day[i].Icon = CacheIcon(cw.Forecast.TxtForecast.Day[i].Icon)
    }
    // Write data to local file
    f,_ := os.Create(filename)
    out,_ := json.Marshal(cw)
    f.Write(out)
    f.Close()
    return cw, nil
}

func ShowCurrent(w http.ResponseWriter, req * http.Request) {
    var err error
    // Find out incoming IP address
    incoming := strings.Split(req.RemoteAddr, ":")[0]
    if incoming=="127.0.0.1" {
        // Handle case where nginx used as reverse proxy
        // This needs to be adapted depending on which header
        // is set with the real requesting IP address.
        incoming = req.Header.Get("X-Real-IP")
    }
    log.Println("incoming", incoming)

    var cw CurrentConditions
    if len(req.URL.Path)>1 {
        // Split request into /country/city
        elems := strings.Split(req.URL.Path[1:], "/")
        lat, lon, _ := Position(elems[0], elems[1])
        cw, err = GetCurrentByPos(lat, lon)
    } else {
        log.Println("req_addr", incoming)
        cw, err = GetCurrentByIP(incoming)
    }
    if err!=nil {
        log.Println(err)
        http.NotFound(w, req)
        return
    }
    w.Header().Set("Content-type", "text/html")
    t, _ := template.ParseFiles("pages/forecast.html")
    t.Execute(w, &cw)
    return
}

// Answer static requests: css/img/js files
func Static(w http.ResponseWriter, req * http.Request) {
    if req.URL.Path=="/favicon.ico" {
        http.ServeFile(w, req, "./static/favicon.ico")
        return
    }
    filename := req.URL.Path[8:]
    http.ServeFile(w, req, "./static/"+filename)
    return
}

func Robots(w http.ResponseWriter, req * http.Request) {
    // Fsck robots
    w.Header().Set("Content-type", "text/plain")
    fmt.Fprintf(w, "User-agent: *\nDisallow: /\n")
}


func main() {
    // Need to obtain an API key from Wunderground
    APIKey = os.Getenv("WUKey")
    if len(APIKey)<1 {
        fmt.Println("Obtain and set WUKey first")
        return
    }
    addr:=Addr
    if len(os.Args)>1 {
        addr=os.Args[1]
    }
    // Setup log file
    logf,_:=os.OpenFile("wunder.log",os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0666)
    defer logf.Close()
    multi:=io.MultiWriter(logf, os.Stderr)
    log.SetOutput(multi)

    // Create resp/ directory if missing
    os.Mkdir(WUResp, 0777)

    // Setup service functions
    http.HandleFunc("/robots.txt", Robots)
    http.HandleFunc("/favicon.ico", Static)
    http.HandleFunc("/static/", Static)
    http.HandleFunc("/", ShowCurrent)

    fmt.Println("Listening on", addr)
    err := http.ListenAndServe(addr, nil)
    if err!=nil {
        fmt.Println(err)
    }
}

