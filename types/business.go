package types

// ProductImage represents an image of a catalog product.

type ProductImage struct {
	URL  string
	Data []byte
}

// ProductCreate contains the data required to create a catalog product.
type ProductCreate struct {
	Name        string
	Description string
	RetailerID  string
	Price       int64
	Currency    string
	IsHidden    bool
	Images      []ProductImage
}

// ProductImageURLs holds the image URLs returned by WhatsApp for a product.
type ProductImageURLs struct {
	Requested string
	Original  string
}

// Product represents a catalog product parsed from a WhatsApp response.
type Product struct {
	ID           string
	Name         string
	Description  string
	RetailerID   string
	URL          string
	Price        int64
	Currency     string
	IsHidden     bool
	ReviewStatus string
	ImageURLs    ProductImageURLs
}

// CatalogStatus contains the review status of a catalog collection.
type CatalogStatus struct {
	Status    string
	CanAppeal bool
}

// Collection represents a catalog collection and its products.
type Collection struct {
	ID       string
	Name     string
	Products []Product
	Status   CatalogStatus
}
