package kindle

import (
	"fmt"
	"time"
)

// ResourceType representa el tipo de recurso de un item de la biblioteca.
type ResourceType string

const (
	ResourceTypeEbook       ResourceType = "EBOOK"
	ResourceTypeEbookSample ResourceType = "EBOOK_SAMPLE"
)

// BookType clasifica el libro según la relación del usuario con él.
type BookType string

const (
	BookTypeOwned   BookType = "owned"
	BookTypeSample  BookType = "sample"
	BookTypeUnknown BookType = "unknown"
)

// OriginType representa el origen de adquisición del libro.
type OriginType string

const (
	OriginTypeKindleUnlimited OriginType = "KINDLE_UNLIMITED"
	OriginTypePrime           OriginType = "PRIME"
	OriginTypeComicsUnlimited OriginType = "COMICS_UNLIMITED"
)

// SortType define el criterio de ordenación para listar la biblioteca.
type SortType string

const (
	SortTypeRecency         SortType = "recency"
	SortTypeTitle           SortType = "title"
	SortTypeAuthor          SortType = "author"
	SortTypeAcquisitionDesc SortType = "acquisition_desc"
	SortTypeAcquisitionAsc  SortType = "acquisition_asc"
)

// Cookies contiene las cuatro cookies de sesión requeridas por Amazon.
// Se obtienen manualmente desde el navegador en read.amazon.com.
type Cookies struct {
	UbidMain  string
	AtMain    string
	SessionID string
	XMain     string
}

// Book representa un item básico de la biblioteca Kindle.
type Book struct {
	Title            string       `json:"title"`
	ASIN             string       `json:"asin"`
	Authors          []string     `json:"authors"`
	MangaOrComicAsin bool         `json:"mangaOrComicAsin"`
	ResourceType     ResourceType `json:"resourceType"`
	OriginType       string       `json:"originType"`
	ProductURL       string       `json:"productUrl"`
	WebReaderURL     string       `json:"webReaderUrl"`
}

// ReadingProgress representa el estado de lectura del libro en un dispositivo.
type ReadingProgress struct {
	ReportedOnDevice string    `json:"reportedOnDevice"`
	Position         int       `json:"position"`
	SyncDate         time.Time `json:"syncDate"`
}

// BookLightDetails extiende Book con información de lectura y metadatos básicos.
type BookLightDetails struct {
	Book
	BookType      BookType        `json:"bookType"`
	FormatVersion string          `json:"formatVersion"`
	Progress      ReadingProgress `json:"progress"`
	LargeCoverURL string          `json:"largeCoverUrl"`
	MetadataURL   string          `json:"metadataUrl"`
	SRL           int             `json:"srl"`
}

// BookDetails extiende BookLightDetails con metadatos del editor y posición de lectura.
type BookDetails struct {
	BookLightDetails
	Publisher      string  `json:"publisher,omitempty"`
	ReleaseDate    string  `json:"releaseDate"`
	StartPosition  int     `json:"startPosition"`
	EndPosition    int     `json:"endPosition"`
	PercentageRead float64 `json:"percentageRead"`
}

// BooksQueryOptions parametriza la consulta de la biblioteca.
type BooksQueryOptions struct {
	SortType        SortType   `json:"sortType,omitempty"`
	OriginType      OriginType `json:"originType,omitempty"`
	PaginationToken string     `json:"paginationToken,omitempty"`
	QuerySize       int        `json:"querySize,omitempty"`
	FetchAllPages   bool       `json:"-"`
}

// DeviceInfo contiene información del dispositivo Kindle registrado.
type DeviceInfo struct {
	ClientHashID       string `json:"clientHashId"`
	DeviceName         string `json:"deviceName"`
	DeviceSessionToken string `json:"deviceSessionToken"`
	EID                string `json:"eid"`
}

// KaramelToken es el token DRM temporal obtenido al iniciar la lectura de un libro.
type KaramelToken struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expiresAt"`
}

// APIError representa un error HTTP de la API de Amazon.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("kindle API error %d: %s", e.StatusCode, e.Message)
}

// --- Tipos internos para parsear respuestas de la API (no exportados) ---

type lastPageReadData struct {
	DeviceName string `json:"deviceName"`
	Position   int    `json:"position"`
	SyncTime   int64  `json:"syncTime"`
}

type startReadingResponse struct {
	YJFormatVersion  string           `json:"YJFormatVersion"`
	ContentType      string           `json:"contentType"`
	ContentVersion   string           `json:"contentVersion"`
	DeliveredASIN    string           `json:"deliveredAsin"`
	Format           string           `json:"format"`
	FormatVersion    string           `json:"formatVersion"`
	HasAnnotations   bool             `json:"hasAnnotations"`
	IsOwned          bool             `json:"isOwned"`
	IsSample         bool             `json:"isSample"`
	KaramelToken     KaramelToken     `json:"karamelToken"`
	KindleSessionID  string           `json:"kindleSessionId"`
	LastPageReadData lastPageReadData `json:"lastPageReadData"`
	MetadataURL      string           `json:"metadataUrl"`
	OriginType       string           `json:"originType"`
	RequestedASIN    string           `json:"requestedAsin"`
	SRL              int              `json:"srl"`
}

type bookMetadataResponse struct {
	ACR           string   `json:"ACR"`
	ASIN          string   `json:"asin"`
	StartPosition int      `json:"startPosition"`
	EndPosition   int      `json:"endPosition"`
	ReleaseDate   string   `json:"releaseDate"`
	Title         string   `json:"title"`
	Version       string   `json:"version"`
	Sample        bool     `json:"sample"`
	AuthorList    []string `json:"authorList"`
	Publisher     string   `json:"publisher"`
}

type booksListResponse struct {
	ItemsList []struct {
		Book
		PercentageRead float64 `json:"percentageRead,omitempty"`
	} `json:"itemsList"`
	PaginationToken string `json:"paginationToken"`
}
