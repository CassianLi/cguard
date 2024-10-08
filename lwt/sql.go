package lwt

// SQL
const (
	QueryLwtData string = `SELECT sca.customs_id,
       sca.item_number,
       sca.product_no,
       sca.country,
       IFNULL(scvp.hs_code, sca.hs_code) AS hs_code,
       sca.quantity,
       sca.number_of_package,
       sca.shipping_marks,
       bd.description,
       bd.web_link,
       scvp.sales_channel,
       scvp.declare_country,
       scvp.transport_type,
       scvp.net_weight,
       scvp.length,
       scvp.width,
       scvp.height,
       scvp.volume,
       scvp.price,
       scvp.price_screenshot,
       scvp.eu_vat_rate,
       scvp.vat_amount,
       scvp.referral_fee_rate,
       scvp.referral_fee,
       scvp.processing_fee_rate,
       scvp.interchangeable_fee_rate,
       scvp.authorisation_fee,
       scvp.high_volume_listing_fee,
       scvp.advertising_fee,
       scvp.closing_fee,
       scvp.fulfilment_fee,
       scvp.storage_fee_rate,
	   scvp.storage_fee,
       scvp.ecp_fees,
       if(sca.freight_within_eu_unit>0,sca.freight_within_eu_unit,scvp.within_fee_rate) AS within_fee_rate,
       scvp.outside_fee_rate,
       scvp.delivery_rate,
       scvp.clearance_rate,
       scvp.ground_fee_rate,
       scvp.warehouse_fee_rate,
       scvp.subtotal,
       scvp.profit_rate,
       scvp.profit,
       scvp.eu_duty_rate,
       scvp.customs_value_include_duty,
       scvp.customs_value,
       scvp.final_declared_value
FROM service_customs_article sca
         LEFT JOIN service_declare_value_process scvp ON sca.declare_value_process_id = scvp.id
         LEFT JOIN base_description bd ON scvp.description_id = bd.id
WHERE sca.customs_id =? ORDER BY sca.item_number;`

	// QueryBriefLwtData Query the rows data for brief LWT
	QueryBriefLwtData = `SELECT sca.customs_id,
       sca.item_number,
       sca.product_no,
       sca.country,
       IFNULL(scvp.hs_code, sca.hs_code) AS hs_code,
       sca.quantity,
       sca.number_of_package,
       sca.shipping_marks,
       bd.description,
       bd.web_link,
       scvp.sales_channel,
       scvp.declare_country,
       scvp.transport_type,
       scvp.net_weight,
       scvp.length,
       scvp.width,
       scvp.height
FROM service_customs_article sca
         LEFT JOIN service_customs_value_process scvp ON sca.customs_value_process_id = scvp.id
         LEFT JOIN base_description bd ON scvp.description_id = bd.id
WHERE sca.customs_id =?`

	// QueryPlatAndBillNo SQL Query plat no and bill number for customs
	QueryPlatAndBillNo string = `SELECT c.customs_id, 
       c.plato_no, 
       bb.bill_no
FROM base_customs c
         INNER JOIN service_bill_customs sbc ON c.customs_id = sbc.customs_id
         INNER JOIN base_bill bb ON sbc.bill_id = bb.bill_id
WHERE c.customs_id = ?;`

	// QueryFirstTrackingNumber Query the first tracking_no for customs
	QueryFirstTrackingNumber string = `SELECT MIN(index_no) as index_no,
       tracking_no
FROM base_reference_tracking
WHERE customs_id =?`

	// QueryCustomsBaseInfo 查询报关单的基本信息，申报国家，销售渠道。用来选择对应的模版文件
	QueryCustomsBaseInfo string = `SELECT customs_id,
       declare_country,
       sales_channel
FROM base_customs
WHERE customs_id =?;`

	// QueryCustomsSplitTotal 主单号拆单报关的字报关单总数
	QueryCustomsSplitTotal string = `SELECT COUNT(*)
FROM service_customs_supply
WHERE customs_id =?`

	// QueryCustomsSplitChild 查询主单号拆单报关的子报关单号
	QueryCustomsSplitChild string = `SELECT supply_customs_id
FROM service_customs_supply
WHERE customs_id = ?;`

	// QueryLwtDataForSplit  使用拆单子单号查询，将父单号和父单号的item_number查询出来
	QueryLwtDataForSplit string = `SELECT scsa.customs_id,
       scsa.item                                                                            AS item_number,
       sca.product_no,
       sca.country,
       IFNULL(scvp.hs_code, sca.hs_code)                                                    AS hs_code,
       sca.quantity,
       sca.number_of_package,
       sca.shipping_marks,
       bd.description,
       bd.web_link,
       scvp.sales_channel,
       scvp.declare_country,
       scvp.transport_type,
       scvp.net_weight,
       scvp.length,
       scvp.width,
       scvp.height,
       scvp.volume,
       scvp.price,
       scvp.price_screenshot,
       scvp.eu_vat_rate,
       scvp.vat_amount,
       scvp.referral_fee_rate,
       scvp.referral_fee,
       scvp.processing_fee_rate,
       scvp.interchangeable_fee_rate,
       scvp.authorisation_fee,
       scvp.high_volume_listing_fee,
       scvp.advertising_fee,
       scvp.closing_fee,
       scvp.fulfilment_fee,
       scvp.storage_fee_rate,
       scvp.storage_fee,
       scvp.ecp_fees,
       IF(sca.freight_within_eu_unit > 0, sca.freight_within_eu_unit, scvp.within_fee_rate) AS within_fee_rate,
       scvp.outside_fee_rate,
       scvp.delivery_rate,
       scvp.clearance_rate,
       scvp.ground_fee_rate,
       scvp.warehouse_fee_rate,
       scvp.subtotal,
       scvp.profit_rate,
       scvp.profit,
       scvp.eu_duty_rate,
       scvp.customs_value_include_duty,
       scvp.customs_value,
       scvp.final_declared_value
FROM service_customs_article sca
         INNER JOIN service_customs_supply_article scsa ON sca.id = scsa.article_id
         LEFT JOIN service_declare_value_process scvp ON sca.declare_value_process_id = scvp.id
         LEFT JOIN base_description bd ON scvp.description_id = bd.id
WHERE sca.customs_id = ?
ORDER BY scsa.item;`

	// QueryBriefLwtDataForSplit 拆单报关，通过子单号插叙父单号和父单号的item_number
	QueryBriefLwtDataForSplit string = `SELECT scsa.customs_id,
       scsa.item AS item_number,
       sca.product_no,
       sca.country,
       IFNULL(scvp.hs_code, sca.hs_code) AS hs_code,
       sca.quantity,
       sca.number_of_package,
       sca.shipping_marks,
       bd.description,
       bd.web_link,
       scvp.sales_channel,
       scvp.declare_country,
       scvp.transport_type,
       scvp.net_weight,
       scvp.length,
       scvp.width,
       scvp.height
FROM service_customs_article sca
         INNER JOIN service_customs_supply_article scsa ON sca.id = scsa.article_id
         LEFT JOIN service_customs_value_process scvp ON sca.customs_value_process_id = scvp.id
         LEFT JOIN base_description bd ON scvp.description_id = bd.id
WHERE sca.customs_id = ?
ORDER BY scsa.item;`
)
