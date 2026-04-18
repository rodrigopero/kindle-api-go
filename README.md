# kindle-go

> Go client for Kindle's unofficial API.

[![Go Reference](https://pkg.go.dev/badge/github.com/rodrigopero/kindle-api-go.svg)](https://pkg.go.dev/github.com/rodrigopero/kindle-api-go)
[![MIT License](https://img.shields.io/badge/license-MIT-blue)](./LICENSE)

This is a Go port of [kindle-api](https://github.com/transitive-bullshit/kindle-api) by [Travis Fischer](https://x.com/transitive_bs).

- [Intro](#intro)
- [Install](#install)
- [Setup](#setup)
  - [TLS Server](#tls-server)
  - [Cookies](#cookies)
  - [Device Token](#device-token)
- [Usage](#usage)
  - [Book Details](#book-details)
  - [Book Content Manifest](#book-content-manifest)
  - [Client Options](#client-options)
- [Type Reference](#type-reference)
- [Disclaimer](#disclaimer)
- [License](#license)

## Intro

Provides an idiomatic Go client for accessing Amazon Kindle's private API: book library, metadata, and content manifests.

> [!IMPORTANT]
> This library is not officially supported by Amazon / Kindle. Using this library might violate Kindle's Terms of Service. Use it at your own risk.

## Install

```sh
go get github.com/rodrigopero/kindle-api-go
```

Requires Go 1.21+.

## Setup

### TLS Server

Amazon applies strict TLS fingerprinting. This library requires a local [tls-client-api](https://github.com/bogdanfinn/tls-client-api) server to spoof Chrome's TLS fingerprint.

<details>
<summary>Example <code>config.dist.yml</code></summary>

The only change from the defaults is `api_auth_keys`, which you need to set as the `TLS_SERVER_API_KEY` environment variable.

```yml
env: dev

app_project: tls-client
app_family: tls-client
app_name: api

log:
  handlers:
    main:
      formatter: console
      level: info
      timestamp_format: '15:04:05:000'
      type: iowriter
      writer: stdout

api:
  port: 8080
  mode: release
  timeout:
    read: 120s
    write: 120s
    idle: 120s

api_auth_keys: ['your-api-key']

api_cors_allowed_headers: ['X-API-KEY', 'Content-Type']
api_cors_allowed_methods: ['POST', 'GET']
```

</details>

### Cookies

Amazon's login system is strict and SMS 2FA makes automating logins difficult. Instead, this library uses 4 cookies that stay valid for an entire year:

- `ubid-main`
- `at-main`
- `x-main`
- `session-id`

Open DevTools on [read.amazon.com](https://read.amazon.com), go to the **Network** tab, and copy the `Cookie` header from any request.

![cookies in the network panel](./kindle-api/assets/cookie-demonstration.png)

### Device Token

You also need the `deviceToken` for your Kindle. Find it in the same Network tab, on the request to:

```
https://read.amazon.com/service/web/register/getDeviceToken?serialNumber=(your-token)&deviceType=(your-token)
```

![device token network request](./kindle-api/assets/kindle-device-token.png)

Both parameters (`serialNumber` and `deviceType`) have the same value.

## Usage

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"

    kindle "github.com/rodrigopero/kindle-api-go"
)

func main() {
    cookies, err := kindle.DeserializeCookies("ubid-main=xxx; at-main=xxx; session-id=xxx; x-main=xxx")
    if err != nil {
        log.Fatal(err)
    }

    client, err := kindle.NewClient(
        cookies,
        "your-device-token",
        "http://127.0.0.1:8080",
        "your-tls-server-api-key",
    )
    if err != nil {
        log.Fatal(err)
    }

    ctx := context.Background()

    // Initialize session and fetch the full book library
    if err := client.Init(ctx); err != nil {
        log.Fatal(err)
    }

    out, _ := json.MarshalIndent(client.Books, "", "  ")
    fmt.Println(string(out))
}

/*
[
  {
    "title": "Revelation Space (The Inhibitor Trilogy Book 1)",
    "asin": "B0819W19WD",
    "webReaderUrl": "https://read.amazon.com/?asin=B0819W19WD",
    "productUrl": "https://m.media-amazon.com/images/I/41sMaof0iQL._SY400_.jpg",
    "authors": ["Alastair Reynolds"],
    "resourceType": "EBOOK",
    "originType": "PURCHASE",
    "mangaOrComicAsin": false
  },
  ...
]
*/
```

You can also pass configuration via environment variables:

```sh
export KINDLE_COOKIES='ubid-main=... at-main=... session-id=... x-main=...'
export KINDLE_DEVICE_TOKEN='...'
export TLS_SERVER_URL='http://127.0.0.1:8080'
export TLS_SERVER_API_KEY='...'
```

```go
cookies, _ := kindle.DeserializeCookies(os.Getenv("KINDLE_COOKIES"))
client, _ := kindle.NewClient(
    cookies,
    os.Getenv("KINDLE_DEVICE_TOKEN"),
    os.Getenv("TLS_SERVER_URL"),
    os.Getenv("TLS_SERVER_API_KEY"),
)
```

To run the included example:

```sh
cd examples/basic
KINDLE_COOKIES="..." KINDLE_DEVICE_TOKEN="..." TLS_SERVER_URL="..." TLS_SERVER_API_KEY="..." go run main.go
```

### Book Details

```go
details, err := client.GetBookDetails(ctx, client.Books[0].ASIN)
if err != nil {
    log.Fatal(err)
}

out, _ := json.MarshalIndent(details, "", "  ")
fmt.Println(string(out))

/*
{
  "title": "Revelation Space (The Inhibitor Trilogy Book 1)",
  "asin": "B0819W19WD",
  "authors": ["Alastair Reynolds"],
  "bookType": "owned",
  "formatVersion": "CR!WPPV87W8317H7FWJRF6JFMVE7SJY",
  "mangaOrComicAsin": false,
  "originType": "PURCHASE",
  "productUrl": "https://m.media-amazon.com/images/I/41sMaof0iQL._SY400_.jpg",
  "largeCoverUrl": "https://m.media-amazon.com/images/I/41sMaof0iQL.jpg",
  "progress": {
    "reportedOnDevice": "My Kindle",
    "position": 163586,
    "syncDate": "2024-10-04T06:28:56Z"
  },
  "webReaderUrl": "https://read.amazon.com/?asin=B0819W19WD",
  "srl": 2393,
  "percentageRead": 13.1,
  "releaseDate": "21/04/2020",
  "startPosition": 5,
  "endPosition": 1250310,
  "publisher": "Orbit"
}
*/
```

### Book Content Manifest

Kindle uses heavy DRM for actual book content. However, it is possible to retrieve rendering manifest data. This method returns a TAR file as a binary-encoded string. Unzipping the TAR produces about a dozen JSON files with the book's pre-rendered content, layout, TOC, and metadata.

```go
manifestTar, err := client.GetBookContentManifest(ctx, client.Books[0].ASIN)
if err != nil {
    log.Fatal(err)
}
// manifestTar is a string with the binary TAR content
```

### Client Options

```go
client, err := kindle.NewClient(
    cookies,
    deviceToken,
    tlsServerURL,
    tlsServerAPIKey,
    kindle.WithThrottle(true),                  // rate limit to 3 req/s (default: true)
    kindle.WithBaseURL("https://..."),           // override Amazon base URL
    kindle.WithClientVersion("20000100"),        // override Kindle client version
    kindle.WithTLSProxyTimeout(30*time.Second),  // override TLS proxy timeout
)
```

## Type Reference

```go
type Cookies struct {
    UbidMain  string
    AtMain    string
    SessionID string
    XMain     string
}

type Book struct {
    Title            string
    ASIN             string
    Authors          []string
    MangaOrComicAsin bool
    ResourceType     ResourceType  // "EBOOK" | "EBOOK_SAMPLE"
    OriginType       string
    ProductURL       string
    WebReaderURL     string
}

type BookDetails struct {
    Book
    BookType       BookType   // "owned" | "sample" | "unknown"
    FormatVersion  string
    Progress       ReadingProgress
    LargeCoverURL  string
    MetadataURL    string
    Publisher      string
    ReleaseDate    string
    StartPosition  int
    EndPosition    int
    PercentageRead float64
}
```

## Disclaimer

This library is not endorsed or supported by Amazon / Kindle. It is an unofficial library intended for educational purposes and personal use only. By using this library, you agree to not hold the author or contributors responsible for any consequences resulting from its usage.

## License

MIT
