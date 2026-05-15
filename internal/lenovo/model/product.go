package model

// LenovoLaptop representa um notebook Lenovo após enriquecimento com a página do produto.
type LenovoLaptop struct {
	Name          string  `json:"name"`
	Price         string  `json:"price"`
	PriceUSD      float64 `json:"price_usd"`
	Currency      string  `json:"currency"`
	Description   string  `json:"description"`
	Rating        int     `json:"rating"`
	ReviewCount   int     `json:"review_count"`
	ProductURL    string  `json:"product_url"`
	ImageURL      string  `json:"image_url,omitempty"`
	HDDOptionsGB  []int   `json:"hdd_options_gb,omitempty"`
	HDDSelectedGB *int    `json:"hdd_selected_gb,omitempty"`
	ProductID     string  `json:"product_id,omitempty"`
}

// ListingsSummary é um item mínimo extraído da listagem (antes do GET na página do produto).
type ListingsSummary struct {
	Name        string
	Price       string
	Description string
	Rating      int
	ReviewCount int
	RelativeURL string
}
