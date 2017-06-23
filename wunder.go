//
// Get weather forecast for any IP
// Weather data from wunderground.com
// Results are served by an HTTP server
//
package main

import (
    "encoding/json"
    "errors"
    "fmt"
    "html/template"
    "io"
    "io/ioutil"
    "log"
    "net/http"
    "net/url"
    "os"
    "os/signal"
    "path"
    "strconv"
    "strings"
    "syscall"
    "time"
)

const (
    WUResp          = "resp/"
    Addr            = ":8001"
)

var (
    APIKey  string
)

// Need to remember how many calls were made to various APIs
// to avoid overflooding them.
type Stamp struct {
    Hits    int             // How many hits recorded over last period
    Start   time.Time       // Start of last period
    MaxHits int             // Max hits to observe
    Period  time.Duration   // Period duration
}

// Initialize a stamper
func (ts *Stamp) Init(MaxHits int, Period time.Duration) {
    ts.MaxHits = MaxHits
    ts.Period  = Period
    ts.Start   = time.Now()
    ts.Hits    = 0
}

// Register a hit
// If at least MaxHits have been recorded within the past Period, return
// False.
func (ts *Stamp) Hit() bool {
    // Examine Stamp: if more than Period time has elapsed, reset stamp
    // and register one hit
    if time.Since(ts.Start) > ts.Period {
        ts.Start = time.Now()
        ts.Hits = 1
        return true
    }
    ts.Hits += 1
    if ts.Hits > ts.MaxHits {
        return false
    }
    return true
}
// Save state to JSON file
func (ts *Stamp) Save(filename string) {
    b, _ := json.Marshal(ts)
    ioutil.WriteFile(filename, b, 0644)
}

// Restore state from JSON file
func (ts *Stamp) Load(filename string, MaxHits int, Period time.Duration) {
    f, err := os.Open(filename)
    if err!=nil {
        ts.Init(MaxHits, Period)
        return
    }
    b, err := ioutil.ReadAll(f)
    f.Close()
    if err!=nil {
        ts.Init(MaxHits, Period)
        return
    }
    json.Unmarshal(b, ts)
}

// To absorb data from ipinfo.io
type GeoIP struct {
    // Country     string  `json:"country"`
    CountryCode string  `json:"country"`
    City        string  `json:"city"`
    Loc         string  `json:"loc"`
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

var APILimits struct {
    // ipapi   Stamp
    wunder  Stamp
    maps    Stamp
}

// Geoloc an IP address, return city + country
func Geolocalize(addr string) (geo GeoIP, err error) {
/*
    if APILimits.ipapi.Hit() != true {
        fmt.Println("** ip-api usage limit exceeded")
        err = errors.New("API usage limit exceeded")
        return
    }
*/
    resp, err := http.Get("http://ipinfo.io/" + addr + "/json")
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
    if APILimits.maps.Hit()!=true {
        err = errors.New("API limit exceeded")
        return
    }
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
    if APILimits.wunder.Hit()!=true {
        err = errors.New("API limit exceeded")
        return
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
             fmt.Sprintf("%s.json", geo.Loc)
    }
    if APILimits.wunder.Hit()!=true {
        return cw, errors.New("API limit exceeded")
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
    log.Println("incoming", incoming, "req", req.URL.Path)

    // Kick bots out
    if strings.Contains(req.URL.Path, ".php") ||
        strings.Contains(req.URL.Path, "xml") ||
        strings.Contains(req.URL.Path, "html") {
        http.NotFound(w, req)
        return
    }

    var cw CurrentConditions
    if len(req.URL.Path)>1 {
        // Split request into /country/city
        elems := strings.Split(req.URL.Path[1:], "/")
        if len(elems)!=2 {
            log.Println("malformed req:", req.URL.Path)
            http.Error(w, "Malformed request: "+req.URL.Path, 400)
            return
        }
        lat, lon, err := Position(elems[0], elems[1])
        if err!=nil {
            log.Println("cannot locate:", elems[0], elems[1], err)
            http.Error(w, err.Error(), 503)
            return
        }
        cw, err = GetCurrentByPos(lat, lon)
        if err!=nil {
            log.Println(err)
            http.Error(w, err.Error(), 503)
            return
        }
    } else {
        log.Println("req_addr", incoming)
        cw, err = GetCurrentByIP(incoming)
    }
    if err!=nil {
        log.Println(err)
        http.Error(w, err.Error(), 503)
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

    // Initialize time stamps for API calls
    // APILimits.ipapi.Load("ts-ipinfo.json", 120, time.Minute)
    APILimits.wunder.Load("ts-wunder.json", 1, 3*time.Minute)
    APILimits.maps.Load("ts-maps.json", 1, 1*time.Minute)

    // Install signal handler for interrupts
    sigs := make(chan os.Signal, 1)
    signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
    go func() {
        <-sigs
        // APILimits.ipapi.Save("ts-ipapi.json")
        APILimits.wunder.Save("ts-wunder.json")
        APILimits.maps.Save("ts-maps.json")
        os.Exit(0)
    }()

    fmt.Println("Listening on", addr)
    err := http.ListenAndServe(addr, nil)
    if err!=nil {
        fmt.Println(err)
    }
}

