package lwt

// SQL
const (
	QueryLwtData string = `SELECT sca.customs_id,
       sca.item_number,
       sca.product_no,
       sca.country,
       sca.hs_code,
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
       scvp.processing_fee_rate,
       scvp.interchangeable_fee_rate,
       scvp.authorisation_fee,
       scvp.high_volume_listing_fee,
       scvp.advertising_fee,
       scvp.closing_fee,
       scvp.fulfilment_fee,
       scvp.storage_fee_rate,
       scvp.ecp_fees,
       scvp.within_fee_rate,
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
         LEFT JOIN service_customs_value_process scvp ON sca.customs_value_process_id = scvp.id
         LEFT JOIN base_description bd ON scvp.description_id = bd.id
WHERE sca.customs_id =? ORDER BY sca.item_number;`
)
