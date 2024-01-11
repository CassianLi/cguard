package lwt

import "database/sql"

// ExcelColumnForLwt customs value Excel data for LWT
type ExcelColumnForLwt struct {
	CustomsId               string  `db:"customs_id"`
	ItemNumber              string  `db:"item_number"`
	ProductNo               string  `db:"product_no"`
	Country                 string  `db:"country"`
	HsCode                  string  `db:"hs_code"`
	Quantity                string  `db:"quantity"`
	NumberOfPackage         string  `db:"number_of_package"`
	ShippingMarks           string  `db:"shipping_marks"`
	Description             string  `db:"description"`
	WebLink                 string  `db:"web_link"`
	SalesChannel            string  `db:"sales_channel"`
	DeclareCountry          string  `db:"declare_country"`
	TransportType           string  `db:"transport_type"`
	NetWeight               float64 `db:"net_weight"`
	Length                  float64 `db:"length"`
	Width                   float64 `db:"width"`
	Height                  float64 `db:"height"`
	Volume                  float64 `db:"volume"`
	Price                   float64 `db:"price"`
	PriceScreenshot         string  `db:"price_screenshot"`
	EuVatRate               float64 `db:"eu_vat_rate"`
	VatAmount               float64 `db:"vat_amount"`
	ReferralFeeRate         float64 `db:"referral_fee_rate"`
	ReferralFee             float64 `db:"referral_fee"`
	ProcessingFeeRate       float64 `db:"processing_fee_rate"`
	InterchangeableFeeRate  float64 `db:"interchangeable_fee_rate"`
	AuthorisationFee        float64 `db:"authorisation_fee"`
	HighVolumeListingFee    float64 `db:"high_volume_listing_fee"`
	AdvertisingFee          float64 `db:"advertising_fee"`
	ClosingFee              float64 `db:"closing_fee"`
	FulfilmentFee           float64 `db:"fulfilment_fee"`
	StorageFeeRate          float64 `db:"storage_fee_rate"`
	StorageFee              float64 `db:"storage_fee"`
	EcpFees                 float64 `db:"ecp_fees"`
	WithinFeeRate           float64 `db:"within_fee_rate"`
	OutsideFeeRate          float64 `db:"outside_fee_rate"`
	DeliveryRate            float64 `db:"delivery_rate"`
	ClearanceRate           float64 `db:"clearance_rate"`
	GroundFeeRate           float64 `db:"ground_fee_rate"`
	WarehouseFeeRate        float64 `db:"warehouse_fee_rate"`
	Subtotal                float64 `db:"subtotal"`
	ProfitRate              float64 `db:"profit_rate"`
	Profit                  float64 `db:"profit"`
	EuDutyRate              float64 `db:"eu_duty_rate"`
	CustomsValueIncludeDuty float64 `db:"customs_value_include_duty"`
	CustomsValue            float64 `db:"customs_value"`
	FinalDeclaredValue      float64 `db:"final_declared_value"`
}

type EcpFeeRate struct {
	DeclareCountry         sql.NullString `db:"declare_country"`
	SalesChannel           sql.NullString `db:"sales_channel"`
	Country                sql.NullString `db:"country"`
	ProcessingFeeRate      float64        `db:"processing_fee_rate"`
	InterchangeableFeeRate float64        `db:"interchangeable_fee_rate"`
	AuthorisationFee       float64        `db:"authorisation_fee"`
	HighVolumeListingFee   float64        `db:"high_volume_listing_fee"`
	AdvertisingFee         float64        `db:"advertising_fee"`
}

// ExcelColumnForBriefLwt customs value Excel data for LWT
type ExcelColumnForBriefLwt struct {
	CustomsId       string         `db:"customs_id"`
	BillNo          sql.NullString `db:"bill_no"`
	PlatoNo         sql.NullString `db:"plato_no"`
	TrackingNo      sql.NullString `db:"tracking_no"`
	ItemNumber      string         `db:"item_number"`
	ProductNo       string         `db:"product_no"`
	Country         string         `db:"country"`
	HsCode          string         `db:"hs_code"`
	Quantity        string         `db:"quantity"`
	NumberOfPackage string         `db:"number_of_package"`
	ShippingMarks   string         `db:"shipping_marks"`
	Description     string         `db:"description"`
	WebLink         string         `db:"web_link"`
	SalesChannel    string         `db:"sales_channel"`
	DeclareCountry  string         `db:"declare_country"`
	TransportType   string         `db:"transport_type"`
	NetWeight       float64        `db:"net_weight"`
	Length          float64        `db:"length"`
	Width           float64        `db:"width"`
	Height          float64        `db:"height"`
}

type BillNoAndPlatForCustoms struct {
	CustomsId string         `db:"customs_id"`
	BillNo    sql.NullString `db:"bill_no"`
	PlatoNo   sql.NullString `db:"plato_no"`
}

type TrackingNoForCustoms struct {
	IndexNo    string         `db:"index_no"`
	TrackingNo sql.NullString `db:"tracking_no"`
}

// ResponseForLwt response for Lwt request
type ResponseForLwt struct {
	CustomsId   string `json:"customs_id"`
	Status      string `json:"status"`
	LwtFilename string `json:"lwt_filename"`
	Error       string `json:"errors"`
	Brief       bool   `json:"brief"`
}

// RequestForLwt Request for Lwt
type RequestForLwt struct {
	CustomsId string `json:"customs_id"`
	Brief     bool   `json:"brief"`
}
