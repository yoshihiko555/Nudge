package dto

// BrainTemplate は Brain 作成用のテンプレート情報。
type BrainTemplate struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// CreatedPage は作成済みページの最小情報。
type CreatedPage struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}
