package lwt

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
	ProcessingFeeRate       float64 `db:"processing_fee_rate"`
	InterchangeableFeeRate  float64 `db:"interchangeable_fee_rate"`
	AuthorisationFee        float64 `db:"authorisation_fee"`
	HighVolumeListingFee    float64 `db:"high_volume_listing_fee"`
	AdvertisingFee          float64 `db:"advertising_fee"`
	ClosingFee              float64 `db:"closing_fee"`
	FulfilmentFee           float64 `db:"fulfilment_fee"`
	StorageFeeRate          float64 `db:"storage_fee_rate"`
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

// ResponseForLwt response for Lwt request
type ResponseForLwt struct {
	Status      string `json:"status"`
	LwtFilename string `json:"lwt_filename"`
	Error       string `json:"errors"`
}
