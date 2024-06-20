package main

import (
    "encoding/json"
    "fmt"
    "html/template"
    "net/http"
    "time"
    "strconv"
)

type HNHit struct {
    Title     string `json:"title"`
    Points    int    `json:"points"`
    URL       string `json:"url"`
    CreatedAt int64  `json:"created_at_i"`
    ObjectID  string `json:"objectID"`
}

type HNResponse struct {
    Hits []HNHit `json:"hits"`
}

type Story struct {
    Title     string
    Points    int
    URL       string
    TimeAgo   string
    HNURL     string
    ObjectID  string
}

var storiesMap = make(map[string]Story)

func fetchHNTitles() ([]Story, error) {
    currentTimestamp := time.Now().Unix()
    time36HoursAgo := currentTimestamp - 48*60*60
    time12HoursAgo := currentTimestamp - 24*60*60

    apiURL := fmt.Sprintf("https://hn.algolia.com/api/v1/search_by_date?tags=show_hn&&numericFilters=created_at_i%%3E%d,created_at_i%%3C%d&hitsPerPage=100", time36HoursAgo, time12HoursAgo)

    resp, err := http.Get(apiURL)
    if err != nil {
        return nil, fmt.Errorf("error making request: %w", err)
    }
    defer resp.Body.Close()

    var hnResponse HNResponse
    if err := json.NewDecoder(resp.Body).Decode(&hnResponse); err != nil {
        return nil, fmt.Errorf("error parsing response: %w", err)
    }

    var stories []Story
    for _, hit := range hnResponse.Hits {
        story := Story{
            Title:   hit.Title,
            Points:  hit.Points,
            URL:     hit.URL,
            TimeAgo: fmt.Sprintf("%d hours ago", (currentTimestamp-hit.CreatedAt)/3600),
            HNURL:   fmt.Sprintf("https://news.ycombinator.com/shownew?next=%s", hit.ObjectID),
            ObjectID: hit.ObjectID,
        }
        stories = append(stories, story)
        storiesMap[hit.ObjectID] = story
    }

    return stories, nil
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
    stories, err := fetchHNTitles()
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    tmpl, err := template.ParseFiles("templates/index.html")
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    if err := tmpl.Execute(w, stories); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
    }
}


func revealHandler(w http.ResponseWriter, r *http.Request) {
    objectID := r.URL.Query().Get("objectID")
    action := r.URL.Query().Get("action")
    story, exists := storiesMap[objectID]
    if !exists {
        http.Error(w, "Story not found", http.StatusNotFound)
        return
    }

    result := ""
    if action == "bullish" {
        if story.Points > 40 {
						result = strconv.Itoa(story.Points) + ` upvotes` + `<span class="text-green-600">` + ` - your bet: bullish </span>`
        } else {
						result = strconv.Itoa(story.Points) + ` upvotes`+ `<span class="text-red-600">` +  ` - your bet: bullish </span>`
        }
    } else if action == "bearish" {
        if story.Points <= 40 {
						result = strconv.Itoa(story.Points) + ` upvotes` + `<span class="text-green-600">` + ` - your bet: bearish </span>`
						} else {
						result = strconv.Itoa(story.Points) + ` upvotes`+ `<span class="text-red-600">` +  ` - your bet: bearish </span>`
        }
    }

    fmt.Fprintf(w, `<div>%s</div>`, result)
}


func main() {
    http.HandleFunc("/", indexHandler)
    http.HandleFunc("/reveal", revealHandler)
    http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
    fmt.Println("Starting server on :8080")
    if err := http.ListenAndServe(":8080", nil); err != nil {
        fmt.Printf("Could not start server: %s\n", err.Error())
    }
}
