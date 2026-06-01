package model

import "database/sql"

// ExcelColumnForLwt customs value Excel data for LWT
type ExcelColumnForLwt struct {
	CustomsId               string          `db:"customs_id"`
	ItemNumber              string          `db:"item_number"`
	ProductNo               string          `db:"product_no"`
	Country                 string          `db:"country"`
	HsCode                  string          `db:"hs_code"`
	Quantity                string          `db:"quantity"`
	NumberOfPackage         string          `db:"number_of_package"`
	ShippingMarks           string          `db:"shipping_marks"`
	Description             string          `db:"description"`
	WebLink                 string          `db:"web_link"`
	SalesChannel            string          `db:"sales_channel"`
	DeclareCountry          string          `db:"declare_country"`
	TransportType           string          `db:"transport_type"`
	NetWeight               float64         `db:"net_weight"`
	Length                  float64         `db:"length"`
	Width                   float64         `db:"width"`
	Height                  float64         `db:"height"`
	Volume                  float64         `db:"volume"`
	Price                   float64         `db:"price"`
	PriceScreenshot         string          `db:"price_screenshot"`
	EuVatRate               float64         `db:"eu_vat_rate"`
	VatAmount               float64         `db:"vat_amount"`
	ReferralFeeRate         float64         `db:"referral_fee_rate"`
	ReferralFee             sql.NullFloat64 `db:"referral_fee"`
	ProcessingFeeRate       float64         `db:"processing_fee_rate"`
	InterchangeableFeeRate  float64         `db:"interchangeable_fee_rate"`
	AuthorisationFee        sql.NullFloat64 `db:"authorisation_fee"`
	HighVolumeListingFee    sql.NullFloat64 `db:"high_volume_listing_fee"`
	AdvertisingFee          sql.NullFloat64 `db:"advertising_fee"`
	ClosingFee              sql.NullFloat64 `db:"closing_fee"`
	FulfilmentFee           sql.NullFloat64 `db:"fulfilment_fee"`
	StorageFeeRate          float64         `db:"storage_fee_rate"`
	StorageFee              sql.NullFloat64 `db:"storage_fee"`
	EcpFees                 float64         `db:"ecp_fees"`
	WithinFeeRate           float64         `db:"within_fee_rate"`
	OutsideFeeRate          float64         `db:"outside_fee_rate"`
	DeliveryRate            float64         `db:"delivery_rate"`
	ClearanceRate           float64         `db:"clearance_rate"`
	GroundFeeRate           float64         `db:"ground_fee_rate"`
	WarehouseFeeRate        float64         `db:"warehouse_fee_rate"`
	Subtotal                float64         `db:"subtotal"`
	ProfitRate              float64         `db:"profit_rate"`
	Profit                  sql.NullFloat64          `db:"profit"`
	EuDutyRate              float64         `db:"eu_duty_rate"`
	CustomsValueIncludeDuty float64         `db:"customs_value_include_duty"`
	CustomsValue            float64         `db:"customs_value"`
	FinalDeclaredValue      float64         `db:"final_declared_value"`
}

// ExcelColumnForLwtAdjustment 修改LWT时，需要计算的因子
type ExcelColumnForLwtAdjustment struct {
	// 行号
	RowNumber int `json:"row_number"`

	// 调整参数时，仅需要计算的因子
	ItemNumber int `json:"item_number"`

	ProductNo              string  `json:"product_no"`
	Quantity               int     `json:"quantity"`
	NetWeight              float64 `json:"net_weight"`
	Height                 float64 `json:"height"`
	Width                  float64 `json:"width"`
	Length                 float64 `json:"length"`
	Price                  float64 `json:"price"`
	EuVatRate              float64 `json:"eu_vat_rate"`
	ReferralFeeRate        float64 `json:"referral_fee_rate"`
	ProcessingFeeRate      float64 `json:"processing_fee_rate"`
	AuthorisationFee       float64 `json:"authorisation_fee"`
	InterchangeableFeeRate float64 `json:"interchangeable_fee_rate"`
	FulfilmentFee          float64 `json:"fulfilment_fee"`
	StorageFeeRate         float64 `json:"storage_fee_rate"`
	GroundFeeRate          float64 `json:"ground_fee_rate"`
	WarehouseFeeRate       float64 `json:"warehouse_fee_rate"`
	ClearanceRate          float64 `json:"clearance_rate"`
	DeliveryRate           float64 `json:"delivery_rate"`
	EuDutyRate             float64 `json:"eu_duty_rate"`
	WithinFeeRate          float64 `json:"within_fee_rate"`

	// 是否已经调整
	IsAdjusted bool `json:"is_adjusted"`
	// 是否存在差异
	HasDiff bool `json:"has_diff"`

	// 以下字段用于标记需要调整的值
	// FinalDeclaredValueOld 当前申报价值
	FinalDeclaredValueOld float64 `json:"final_declared_value_old"`
	// FinalDeclaredValueTarget 原报关单申报价值（微调的目标值）
	FinalDeclaredValueTarget float64 `json:"final_declared_value_target"`
}

// CustomsBaseInfo customs base info, include customs id, declare country, sales channel
type CustomsBaseInfo struct {
	CustomsId      string `db:"customs_id"`
	DeclareCountry string `db:"declare_country"`
	SalesChannel   string `db:"sales_channel"`
}

// ExcelColumnForLwtSplit ExcelColumnForLwt The customs has been split into multiple sales channels customs value Excel data for LWT
type ExcelColumnForLwtSplit struct {
	ExcelColumnForLwt
	// 拆单报关，通过子单号插叙父单号和父单号的item_number
	ParentCustomsId  string `db:"parent_customs_id"`
	ParentItemNumber string `db:"parent_item_number"`
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
