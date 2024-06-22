package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	"github.com/machinebox/graphql"
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
	Title    string
	Points   int
	URL      string
	TimeAgo  string
	HNURL    string
	ObjectID string
}

type PHPost struct {
	ID         string
	Name       string
	Tagline    string
	TimeAgo    string
	URL        string
	VotesCount int
}

type PageData struct {
	HackerNews  []Story
	ProductHunt []PHPost
}

var storiesMap = make(map[string]Story)
var phPostsMap = make(map[string]PHPost)

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
			Title:    hit.Title,
			Points:   hit.Points,
			URL:      hit.URL,
			TimeAgo:  fmt.Sprintf("%d hours ago", (currentTimestamp-hit.CreatedAt)/3600),
			HNURL:    fmt.Sprintf("https://news.ycombinator.com/shownew?next=%s", hit.ObjectID),
			ObjectID: hit.ObjectID,
		}
		stories = append(stories, story)
		storiesMap[hit.ObjectID] = story
	}

	return stories, nil
}

type loggingTransport struct {
	transport http.RoundTripper
}

func (t *loggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {

	resp, err := t.transport.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Log the raw response body
	fmt.Println("Raw response body:", string(body))

	// Reset the response body so it can be read again
	resp.Body = ioutil.NopCloser(bytes.NewBuffer(body))
	return resp, nil
}

func fetchPHPosts() ([]PHPost, error) {
	token := os.Getenv("PH_APP_CLIENT_CREDENTIALS_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("PH_APP_CLIENT_CREDENTIALS_TOKEN not set in .env file")
	}

	currentTimestamp := time.Now().Unix()
	postedBefore := time.Now().Add(-24 * time.Hour).Format(time.RFC3339)
	postedAfter := time.Now().Add(-48 * time.Hour).Format(time.RFC3339)

	httpClient := &http.Client{
		Transport: &loggingTransport{
			transport: http.DefaultTransport,
		},
	}

	client := graphql.NewClient("https://api.producthunt.com/v2/api/graphql", graphql.WithHTTPClient(httpClient))

	req := graphql.NewRequest(`
        query ($postedAfter: DateTime!, $postedBefore: DateTime!) {
            posts(
                order: NEWEST
                postedAfter: $postedAfter
                postedBefore: $postedBefore
            ) {
                edges {
                    node {
                        id
                        name
                        tagline
                        url
                        createdAt
                        votesCount
                    }
                }
            }
        }
    `)

	req.Var("postedAfter", postedAfter)
	req.Var("postedBefore", postedBefore)
	req.Header.Set("Authorization", "Bearer "+token)

	var response struct {
		Posts struct {
			Edges []struct {
				Node struct {
					ID         string `json:"id"`
					Name       string `json:"name"`
					Tagline    string `json:"tagline"`
					URL        string `json:"url"`
					CreatedAt  string `json:"createdAt"`
					VotesCount int    `json:"votesCount"`
				} `json:"node"`
			} `json:"edges"`
		} `json:"posts"`
	}

	ctx := context.Background()
	if err := client.Run(ctx, req, &response); err != nil {
		client.Log = func(s string) { fmt.Println(s) } // This will log the request and response
		return nil, fmt.Errorf("error making GraphQL request: %w", err)
	}

	var posts []PHPost
	for _, edge := range response.Posts.Edges {
		post := PHPost{
			ID:         edge.Node.ID,
			Name:       edge.Node.Name,
			Tagline:    edge.Node.Tagline,
			URL:        edge.Node.URL,
			VotesCount: edge.Node.VotesCount,
			TimeAgo:    calculateTimeAgo(edge.Node.CreatedAt, currentTimestamp),
		}
		posts = append(posts, post)
		phPostsMap[edge.Node.ID] = post
	}

	return posts, nil
}
func calculateTimeAgo(createdAt string, currentTimestamp int64) string {
	layout := time.RFC3339
	t, err := time.Parse(layout, createdAt)
	if err != nil {
		return "unknown time ago"
	}
	duration := time.Duration(currentTimestamp-t.Unix()) * time.Second
	hours := int(duration.Hours())
	return fmt.Sprintf("%d hours ago", hours)
}
func indexHandler(w http.ResponseWriter, r *http.Request) {
	var hnStories []Story
	var phPosts []PHPost
	var err error

	if cache.isExpired() {
		hnStories, err = fetchHNTitles()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		phPosts, err = fetchPHPosts()
		if err != nil {
			log.Printf("Error fetching Product Hunt posts: %v", err)
			phPosts = nil // Proceed with only Hacker News data
		}

		cache.setCache(hnStories, phPosts)
	} else {
		log.Printf("Using cached data")
		hnStories, phPosts = cache.getCache()
	}

	pageData := PageData{
		HackerNews:  hnStories,
		ProductHunt: phPosts,
	}

	tmpl, err := template.ParseFiles("templates/index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := tmpl.Execute(w, pageData); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func revealHandler(w http.ResponseWriter, r *http.Request) {
	objectID := r.URL.Query().Get("objectID")
	action := r.URL.Query().Get("action")

	var result string
	if story, exists := storiesMap[objectID]; exists {
		if story.Points > 40 && action == "bullish" || story.Points <= 40 && action == "bearish" {
			result = strconv.Itoa(story.Points) + ` upvotes` + `<span class="text-green-600"> - Congrats! </span>!`
		} else {
			result = strconv.Itoa(story.Points) + ` upvotes` + `<span class="text-red-600"> - Better luck next time!</span>`
		}
	} else if post, exists := phPostsMap[objectID]; exists {
		if post.VotesCount > 40 && action == "bullish" || post.VotesCount <= 40 && action == "bearish" {
			result = strconv.Itoa(post.VotesCount) + ` upvotes` + `<span class="text-green-600"> - Congrats! </span>!`
		} else {
			result = strconv.Itoa(post.VotesCount) + ` upvotes` + `<span class="text-red-600"> - Better luck next time!</span>`
		}
	} else {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}

	fmt.Fprintf(w, `<div>%s</div>`, result)
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/reveal", revealHandler)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	fmt.Println("Starting server on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Printf("Could not start server: %s\n", err.Error())
	}
}
