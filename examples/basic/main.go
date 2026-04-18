package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	kindle "github.com/rodrigopero/kindle-api-go"
)

func main() {
	rawCookies := os.Getenv("KINDLE_COOKIES")
	deviceToken := os.Getenv("KINDLE_DEVICE_TOKEN")
	tlsServerURL := os.Getenv("TLS_SERVER_URL")
	tlsServerAPIKey := os.Getenv("TLS_SERVER_API_KEY")

	cookies, err := kindle.DeserializeCookies(rawCookies)
	if err != nil {
		log.Fatalf("invalid cookies: %v", err)
	}

	client, err := kindle.NewClient(cookies, deviceToken, tlsServerURL, tlsServerAPIKey)
	if err != nil {
		log.Fatalf("creating client: %v", err)
	}

	ctx := context.Background()

	if err := client.Init(ctx); err != nil {
		log.Fatalf("init: %v", err)
	}

	out, _ := json.MarshalIndent(client.Books, "", "  ")
	fmt.Println(string(out))

	if len(client.Books) > 0 {
		details, err := client.GetBookDetails(ctx, client.Books[0].ASIN)
		if err != nil {
			log.Fatalf("getBookDetails: %v", err)
		}
		out, _ = json.MarshalIndent(details, "", "  ")
		fmt.Println(string(out))
	}
}
