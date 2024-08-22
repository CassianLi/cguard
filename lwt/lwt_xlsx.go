package lwt

import (
	"encoding/json"
	"errors"
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/xuri/excelize/v2"
	"path/filepath"
	"strconv"
	"strings"
	"sysafari.com/customs/cguard/rabbit"
	"sysafari.com/customs/cguard/utils"
	"time"
)

const (
	InsertRowFirst     = 4
	FloatDecimalPlaces = 6
	TimeLayout         = "20060102150405"
)

// LwtExcelSheetMap  SaleChannel 对应的sheet index
var LwtExcelSheetMap = map[string]int{
	"amazon":    0,
	"ebay":      1,
	"cdiscount": 2,
}

// GenerateLWTExcel generate excel file for LWT
func GenerateLWTExcel(data string) {
	response := &ResponseForLwt{
		Status:      "failed",
		LwtFilename: "",
		Error:       "",
	}
	requestForLwt, err := deserializeRequest(data)
	response.Brief = requestForLwt.Brief

	if err != nil {
		response.Error = fmt.Sprintf("Deserialization of MQ message failed, err:%v", err)
	} else {
		var lwtFilename string
		if requestForLwt.Brief {
			lwtFilename, err = makeBriefLWT(requestForLwt.CustomsId)
		} else {
			lwtFilename, err = makeOfficialLWT(requestForLwt.CustomsId)
		}

		if err != nil {
			fmt.Println("Generate LWT excel failed", err)
			response.Error = fmt.Sprintf("Generate LWT excel failed,err:%v", err)
		} else {
			response.CustomsId = requestForLwt.CustomsId
			response.Status = "success"
			response.LwtFilename = lwtFilename
			response.Error = ""
		}

	}
	// pub to  rabbitmq
	publishLwtResult(response)
}

// deserializeRequest is used to deserialize rabbitmq request
func deserializeRequest(message string) (RequestForLwt, error) {
	log.Infof("Deserialize request: %v", message)

	msg, err := strconv.Unquote(message)
	fmt.Println("msg:", msg)

	req := RequestForLwt{}
	if err != nil {
		err = json.Unmarshal([]byte(message), &req)
	} else {
		err = json.Unmarshal([]byte(msg), &req)
	}
	if err != nil {
		return req, err
	}
	return req, nil
}

func publishLwtResult(res *ResponseForLwt) {
	rbmq := &rabbit.Rabbit{
		Url:          viper.GetString("rabbitmq.url"),
		Exchange:     viper.GetString("rabbitmq.exchange"),
		ExchangeType: viper.GetString("rabbitmq.exchange-type"),
		Queue:        viper.GetString("rabbitmq.queue.lwt-res"),
	}
	log.Infof("Lwt response: %v", res)

	marshal, err := json.Marshal(res)
	if err != nil {
		log.Errorf("Marshel struct to json failed: %v", err)
	} else {
		rabbit.Publish(rbmq, string(marshal))
	}
}

// isSplitCustoms 是否是拆单报关？@param customsId 主报关单号
func isSplitCustoms(customsId string) bool {
	var count int
	err := Db.Get(&count, QueryCustomsSplitTotal, customsId)
	if err != nil {
		fmt.Printf("query customs split total failed, err:%v", err)
		return false
	}
	fmt.Println("split count:", count)
	return count > 0
}

// makeOfficialLWTForNormal 普通的报关单生成LWT（未拆单报关）
func makeOfficialLWTForNormal(customsId string) (string, error) {
	var rows []ExcelColumnForLwt
	err := Db.Select(&rows, QueryLwtData, customsId)
	if err != nil {
		return "", err
	}

	if len(rows) == 0 {
		return "", errors.New("cant not query rows for lwt")
	}

	return generateExcelForOfficialLWT(rows)
}

// makeOfficialLWTForSplit 拆单报关单生成LWT
func makeOfficialLWTForSplit(customsId string) (string, error) {
	// 1. 查询子报关单号
	fmt.Println("1. query customs split child, customsId:", customsId)
	var customsIds []string
	err := Db.Select(&customsIds, QueryCustomsSplitChild, customsId)
	if err != nil {
		return "", errors.New(fmt.Sprintf("query customs split child customsIds failed, err:%v", err))
	}
	// 2. 查询主单号的数据
	fmt.Println("2. query customs base info, customsId:", customsId)
	var customsBaseInfo CustomsBaseInfo
	err = Db.Get(&customsBaseInfo, QueryCustomsBaseInfo, customsId)
	if err != nil {
		return "", errors.New(fmt.Sprintf("query customs base info failed, err:%v", err))
	}

	// 3. 准备LWT文件模版及存放路径
	fmt.Println("3. ready for lwt file(template & save path), customsId:", customsId)
	fileSavePath, err := readyFowLwtFile(customsBaseInfo.DeclareCountry, customsId, "split", false)
	if err != nil {
		return "", errors.New(fmt.Sprintf("ready file for lwt failed, err:%v", err))
	}

	// 4. 查询子单号的LWT 数据，并填充到LWT文件中
	// 有一个子单号的数据为空，就返回错误。不再继续执行
	fmt.Println("4. loop fill lwt excel for split, customsIds:", customsIds)
	for _, id := range customsIds {
		var rows []ExcelColumnForLwt
		err = Db.Select(&rows, QueryLwtDataForSplit, id)
		if err != nil {
			return "", err
		}

		salesChannel := strings.ToLower(rows[0].SalesChannel)
		idx, ok := LwtExcelSheetMap[salesChannel]
		if !ok {
			return "", errors.New(fmt.Sprintf("The map LwtExcelSheetMap dont have the sales channel %s.", salesChannel))
		}

		if customsBaseInfo.DeclareCountry == "BE" {
			err = fillLwtExcelForBe(fileSavePath, rows, idx)
		} else {
			err = fillLwtExcelForNl(fileSavePath, rows, idx)
		}
		if err != nil {
			return "", errors.New(fmt.Sprintf("fill lwt excel failed, err:%v", err))
		}
	}
	// 5. 返回文件名
	fmt.Println("5. return lwt file name: ", fileSavePath)
	return filepath.Base(fileSavePath), nil
}

// makeOfficialLWT Make official LWT Excel file
func makeOfficialLWT(customsId string) (string, error) {
	// Is split into multiple sales channels?
	if isSplitCustoms(customsId) {
		fmt.Println("customsId is split, customsId:", customsId)
		return makeOfficialLWTForSplit(customsId)
	} else {
		fmt.Println("customsId is normal, customsId:", customsId)
		return makeOfficialLWTForNormal(customsId)
	}
}

// makeBriefLwtNormal 普通的简易报关文件LWT
func makeBriefLwtNormal(customsId string) (string, error) {
	var rows []ExcelColumnForBriefLwt
	err := Db.Select(&rows, QueryBriefLwtData, customsId)
	if err != nil {
		return "", errors.New(fmt.Sprintf("query brief lwt data failed, err:%v", err))
	}

	if len(rows) == 0 {
		return "", errors.New("cant not query rows for lwt")
	}

	var billPlat BillNoAndPlatForCustoms
	err = Db.Get(&billPlat, QueryPlatAndBillNo, customsId)
	if err != nil {
		return "", errors.New(fmt.Sprintf("query plat and bill no failed, err:%v", err))
	}

	for i := 0; i < len(rows); i++ {
		row := rows[i]
		row.BillNo = billPlat.BillNo
		row.PlatoNo = billPlat.PlatoNo
		rows[i] = row
	}

	return generateExcelForBriefLWT(rows)
}

// makeBriefLwtForSplit 拆单报关文件简易LWT
func makeBriefLwtForSplit(customsId string) (string, error) {
	// 1. 查询子报关单号
	fmt.Println("1. query customs split child, customsId:", customsId)
	var customsIds []string
	err := Db.Select(&customsIds, QueryCustomsSplitChild, customsId)
	if err != nil {
		return "", errors.New(fmt.Sprintf("query customs split child customsIds failed, err:%v", err))
	}
	// 2. 查询主单号的数据
	fmt.Println("2. query customs base info, customsId:", customsId)
	var customsBaseInfo CustomsBaseInfo
	err = Db.Get(&customsBaseInfo, QueryCustomsBaseInfo, customsId)
	if err != nil {
		return "", errors.New(fmt.Sprintf("query customs base info failed, err:%v", err))
	}
	// 3. 查询主单号的plat和bill no
	fmt.Println("3. query plat and bill no, customsId:", customsId)
	var billPlat BillNoAndPlatForCustoms
	err = Db.Get(&billPlat, QueryPlatAndBillNo, customsId)
	if err != nil {
		return "", errors.New(fmt.Sprintf("query plat and bill no failed, err:%v", err))
	}

	// 4. 准备LWT文件模版及存放路径
	fmt.Println("4. ready for lwt file(template & save path), customsId:", customsId)
	fileSavePath, err := readyFowLwtFile(customsBaseInfo.DeclareCountry, customsId, "split", true)
	if err != nil {
		return "", errors.New(fmt.Sprintf("ready file for lwt failed, err:%v", err))
	}

	// 4. 查询子单号的LWT 数据，并填充到LWT文件中
	// 有一个子单号的数据为空，就返回错误。不再继续执行
	fmt.Println("5. loop fill brief lwt excel for split, customsIds:", customsIds)
	for _, id := range customsIds {
		var rows []ExcelColumnForBriefLwt
		err = Db.Select(&rows, QueryBriefLwtDataForSplit, id)
		if err != nil || len(rows) == 0 {
			return "", errors.New(fmt.Sprintf("query brief lwt data for split failed, err:%v", err))
		}

		salesChannel := strings.ToLower(rows[0].SalesChannel)
		idx, ok := LwtExcelSheetMap[salesChannel]
		if !ok {
			return "", errors.New(fmt.Sprintf("The map LwtExcelSheetMap dont have the sales channel %s.", salesChannel))
		}
		// 填充plat和bill no
		fmt.Println("5. fill plat and bill no, child customsId:", id)
		for i := 0; i < len(rows); i++ {
			row := rows[i]
			row.BillNo = billPlat.BillNo
			row.PlatoNo = billPlat.PlatoNo
			rows[i] = row
		}

		err = fillBriefLwtExcel(fileSavePath, rows, idx)

		if err != nil {
			return "", errors.New(fmt.Sprintf("fill brief lwt excel failed, err:%v", err))
		}
	}
	// 5. 返回文件名
	fmt.Println("5. return brief lwt file name: ", fileSavePath)
	return filepath.Base(fileSavePath), nil
}

// makeBriefLWT
func makeBriefLWT(customsId string) (string, error) {
	// Is split into multiple sales channels?
	if isSplitCustoms(customsId) {
		return makeBriefLwtForSplit(customsId)
	} else {
		return makeBriefLwtNormal(customsId)
	}
}

// GenerateLWTExcel generate excel file for LWT,
// error =nil returns lwt file link(oss)
func generateExcelForOfficialLWT(rows []ExcelColumnForLwt) (string, error) {
	declareCountry := rows[0].DeclareCountry
	customId := rows[0].CustomsId
	salesChannel := rows[0].SalesChannel

	lwtFilePath, err := readyFowLwtFile(declareCountry, customId, salesChannel, false)
	if err != nil {
		return "", errors.New(fmt.Sprintf("Prepare LWT file failed, err:%v", err))
	}

	if "BE" == strings.ToUpper(declareCountry) {
		err = fillLwtExcelForBe(lwtFilePath, rows, 0)
	} else {
		err = fillLwtExcelForNl(lwtFilePath, rows, 0)
	}

	if err != nil {
		return "", errors.New(fmt.Sprintf("Fill LWT excel failed, err:%v", err))
	}

	return filepath.Base(lwtFilePath), nil
}

// GenerateLWTExcel generate excel file for LWT,
// error =nil returns lwt file link(oss)
func generateExcelForBriefLWT(rows []ExcelColumnForBriefLwt) (string, error) {
	declareCountry := rows[0].DeclareCountry
	customId := rows[0].CustomsId
	salesChannel := rows[0].SalesChannel

	lwtFilePath, err := readyFowLwtFile(declareCountry, customId, salesChannel, true)
	if err != nil {
		return "", err
	}

	err = fillBriefLwtExcel(lwtFilePath, rows, 0)
	if err != nil {
		return "", err
	}

	return filepath.Base(lwtFilePath), nil
}

// readyFowLwtFile 准备LWT文件模版和存放路径。如果是拆分报关，salesChannel为split
func readyFowLwtFile(declareCountry, customId, salesChannel string, brief bool) (string, error) {
	var templatePath string
	if brief {
		templatePath = viper.GetString(fmt.Sprintf("lwt.template.brief.%s", strings.ToLower(salesChannel)))
	} else {
		templatePath = viper.GetString(fmt.Sprintf("lwt.template.official.%s.%s", strings.ToLower(declareCountry), strings.ToLower(salesChannel)))
	}

	if templatePath == "" {
		return "", errors.New(fmt.Sprintf("SalesChannel: %s not supports to LWT.", salesChannel))
	}

	if !utils.IsExists(templatePath) {
		return "", errors.New(fmt.Sprintf("Template file: %s does not exist", templatePath))
	}

	tmpDir := viper.GetString("lwt.tmp.dir")
	if !utils.IsDir(tmpDir) && !utils.CreateDir(tmpDir) {
		return "", errors.New(fmt.Sprintf("Crate tmp directory: %s failed !", tmpDir))
	}

	now := time.Now()
	timestamp := now.Format(TimeLayout)
	saveDir := filepath.Join(tmpDir, strconv.Itoa(now.Year()), strconv.Itoa(int(now.Month())))
	if !utils.IsDir(saveDir) && !utils.CreateDir(saveDir) {
		return "", errors.New(fmt.Sprintf("Create save dir: %s failed !", saveDir))
	}
	var lwtFilePath string
	if brief {
		lwtFilePath = filepath.Join(saveDir, fmt.Sprintf("BRIEF_LWT_%s_%s.xlsx", customId, timestamp))
	} else {
		lwtFilePath = filepath.Join(saveDir, fmt.Sprintf("LWT_%s_%s.xlsx", customId, timestamp))
	}

	fmt.Println("lwtFilePath: ", lwtFilePath)

	err := utils.Copy(templatePath, lwtFilePath)
	if err != nil {
		return "", errors.New(fmt.Sprintf("Create lwt file: %s form template file: %s failed!", lwtFilePath, templatePath))
	}

	return lwtFilePath, nil
}

var border = []excelize.Border{
	{Type: "left", Color: "000000", Style: 1},
	{Type: "top", Color: "000000", Style: 1},
	{Type: "bottom", Color: "000000", Style: 1},
	{Type: "right", Color: "000000", Style: 1},
}

var alignment = &excelize.Alignment{
	Vertical:   "center",
	Horizontal: "center",
	WrapText:   true,
}

var font = &excelize.Font{
	Color: "#F00000",
}

// fillLwtExcelForNl fill data to lwt excel file for NL
func fillLwtExcelForNl(lwtFilePath string, rows []ExcelColumnForLwt, sheetIdx int) error {
	f, err := excelize.OpenFile(lwtFilePath)
	if err != nil {
		fmt.Println("fill lwt excel file for nl,open file failed", err)
	}

	defer func() {
		// Close the spreadsheet.
		if err := f.Close(); err != nil {
		}
	}()

	f.SetActiveSheet(sheetIdx)

	sheetName := f.GetSheetName(sheetIdx)

	fmt.Printf("sheetName: %s\n", sheetName)

	styleFormula, err := f.NewStyle(&excelize.Style{Border: border, Alignment: alignment, DecimalPlaces: FloatDecimalPlaces})
	style, err := f.NewStyle(&excelize.Style{Border: border, Alignment: alignment})
	stylePercent, err := f.NewStyle(&excelize.Style{Border: border, Alignment: alignment, NumFmt: 10, Font: font})

	if err != nil {
		log.Errorf("Create excel syle failed: %v", err)
	} else {
		for i := 0; i < len(rows); i++ {
			rowNumber := InsertRowFirst + i

			err = f.InsertRow(sheetName, rowNumber)
			row := rows[i]

			err = addStringCellForSheet(f, sheetName, fmt.Sprintf("A%d", rowNumber), row.ItemNumber, style)
			err = addStringCellForSheet(f, sheetName, fmt.Sprintf("B%d", rowNumber), row.ProductNo, style)
			err = addStringCellForSheet(f, sheetName, fmt.Sprintf("C%d", rowNumber), row.Description, style)
			err = addStringCellForSheet(f, sheetName, fmt.Sprintf("D%d", rowNumber), row.Quantity, style)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("E%d", rowNumber), row.NetWeight, styleFormula)
			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("F%d", rowNumber), row.Height, styleFormula)
			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("G%d", rowNumber), row.Width, styleFormula)
			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("H%d", rowNumber), row.Length, styleFormula)

			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("I%d", rowNumber), fmt.Sprintf("=Round((F%d*G%d*H%d)/1000000,6)", rowNumber, rowNumber, rowNumber), styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("J%d", rowNumber), fmt.Sprintf("=Round(I%d*35.315,6)", rowNumber), styleFormula)

			err = addStringCellForSheet(f, sheetName, fmt.Sprintf("K%d", rowNumber), row.Country, style)
			err = addStringCellForSheet(f, sheetName, fmt.Sprintf("L%d", rowNumber), row.HsCode, style)
			err = addStringCellForSheet(f, sheetName, fmt.Sprintf("M%d", rowNumber), row.WebLink, style)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("N%d", rowNumber), 0.0, styleFormula)
			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("O%d", rowNumber), 0.0, styleFormula)
			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("P%d", rowNumber), row.Price, styleFormula)

			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("Q%d", rowNumber), fmt.Sprintf("=P%d", rowNumber), styleFormula)

			// marketplace
			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("R%d", rowNumber), row.EuVatRate, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("S%d", rowNumber), fmt.Sprintf("=Round(Q%d*(1-1/(1+R%d)), 6)", rowNumber, rowNumber), styleFormula)

			// platform cost
			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("T%d", rowNumber), row.ReferralFeeRate, styleFormula)

			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("U%d", rowNumber), fmt.Sprintf("=T%d", rowNumber), stylePercent)

			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("V%d", rowNumber), fmt.Sprintf("=Round(T%d*Q%d,6)", rowNumber, rowNumber), styleFormula)

			//err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("W%d", rowNumber), row.ClosingFee, styleFormula)
			//err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("X%d", rowNumber), row.HighVolumeListingFee, styleFormula)
			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("W%d", rowNumber), row.ProcessingFeeRate, styleFormula)

			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("X%d", rowNumber), fmt.Sprintf("=Round(W%d*Q%d,6)", rowNumber, rowNumber), styleFormula)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("Y%d", rowNumber), row.AuthorisationFee, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("Z%d", rowNumber), fmt.Sprintf("=Y%d", rowNumber), styleFormula)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AA%d", rowNumber), row.InterchangeableFeeRate, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AB%d", rowNumber), fmt.Sprintf("=Round(AA%d*Q%d,6)", rowNumber, rowNumber), styleFormula)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AC%d", rowNumber), row.FulfilmentFee, styleFormula)
			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AD%d", rowNumber), row.StorageFeeRate, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AE%d", rowNumber), fmt.Sprintf("=Round(AD%d*I%d,6)", rowNumber, rowNumber), styleFormula)

			//err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AH%d", rowNumber), row.AdvertisingFee, styleFormula)

			// profit
			//err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AI%d", rowNumber), row.ProfitRate, styleFormula)
			//
			//profitFormula := fmt.Sprintf("=Round(AI%d*Q%d,6)",
			//	rowNumber, rowNumber)
			//err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AJ%d", rowNumber), profitFormula, styleFormula)

			// local cost
			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AF%d", rowNumber), row.GroundFeeRate, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AG%d", rowNumber), fmt.Sprintf("=Round(AF%d*E%d,6)", rowNumber, rowNumber), styleFormula)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AH%d", rowNumber), row.WarehouseFeeRate, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AI%d", rowNumber), fmt.Sprintf("=Round(AH%d*E%d,6)", rowNumber, rowNumber), styleFormula)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AJ%d", rowNumber), row.ClearanceRate, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AK%d", rowNumber), fmt.Sprintf("=Round(AJ%d*E%d,6)", rowNumber, rowNumber), styleFormula)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AL%d", rowNumber), row.DeliveryRate, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AM%d", rowNumber), fmt.Sprintf("=Round(AL%d*E%d,6)", rowNumber, rowNumber), styleFormula)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AN%d", rowNumber), row.WithinFeeRate, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AO%d", rowNumber), fmt.Sprintf("=Round(AN%d*E%d,6)", rowNumber, rowNumber), styleFormula)

			// subtotal
			subtotalFormula := fmt.Sprintf("=Round(AG%d+AI%d+AK%d+AM%d+AO%d,6)", rowNumber, rowNumber, rowNumber, rowNumber, rowNumber)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AP%d", rowNumber), subtotalFormula, styleFormula)

			// customs value include duty
			customsValueIncludeDutyFormula := fmt.Sprintf("=Round(Q%d-(S%d+V%d+X%d+Z%d+AB%d+AC%d+AE%d+AP%d),6)",
				rowNumber, rowNumber, rowNumber, rowNumber, rowNumber, rowNumber, rowNumber, rowNumber, rowNumber)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AQ%d", rowNumber), customsValueIncludeDutyFormula, styleFormula)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AS%d", rowNumber), row.EuDutyRate, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AT%d", rowNumber), fmt.Sprintf("=AS%d", rowNumber), stylePercent)

			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AR%d", rowNumber), fmt.Sprintf("=Round(AQ%d/(1+AS%d),2)", rowNumber, rowNumber), styleFormula)

			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AU%d", rowNumber), fmt.Sprintf("=Round(AR%d*AS%d,2)", rowNumber, rowNumber), styleFormula)

			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AV%d", rowNumber), fmt.Sprintf("=Round(AR%d*D%d,2)", rowNumber, rowNumber), styleFormula)
			if err != nil {
				return err
			}
		}
	}

	// Save the spreadsheet with the origin path.
	if err = f.Save(); err != nil {
		return err
	}
	return nil
}

// fillLwtExcelForBe fill data to lwt excel file for BE
func fillLwtExcelForBe(lwtFilePath string, rows []ExcelColumnForLwt, sheetIdx int) error {
	f, err := excelize.OpenFile(lwtFilePath)
	if err != nil {
		fmt.Println("fill lwt excel file for be,open file failed", err)
	}

	defer func() {
		// Close the spreadsheet.
		if err := f.Close(); err != nil {
		}
	}()
	f.SetActiveSheet(sheetIdx)

	sheetName := f.GetSheetName(sheetIdx)

	fmt.Printf("sheetName: %s\n", sheetName)

	styleFormula, err := f.NewStyle(&excelize.Style{Border: border, Alignment: alignment, DecimalPlaces: FloatDecimalPlaces})
	style, err := f.NewStyle(&excelize.Style{Border: border, Alignment: alignment})
	stylePercent, err := f.NewStyle(&excelize.Style{Border: border, Alignment: alignment, NumFmt: 10, Font: font})

	if err != nil {
		log.Errorf("Create excel syle failed: %v", err)
	} else {
		for i := 0; i < len(rows); i++ {
			rowNumber := InsertRowFirst + i

			err = f.InsertRow(sheetName, rowNumber)
			row := rows[i]

			err = addStringCellForSheet(f, sheetName, fmt.Sprintf("A%d", rowNumber), row.ItemNumber, style)
			err = addStringCellForSheet(f, sheetName, fmt.Sprintf("B%d", rowNumber), row.ProductNo, style)
			err = addStringCellForSheet(f, sheetName, fmt.Sprintf("C%d", rowNumber), row.Description, style)
			err = addStringCellForSheet(f, sheetName, fmt.Sprintf("D%d", rowNumber), row.Quantity, style)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("E%d", rowNumber), row.NetWeight, styleFormula)
			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("F%d", rowNumber), row.Height, styleFormula)
			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("G%d", rowNumber), row.Width, styleFormula)
			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("H%d", rowNumber), row.Length, styleFormula)

			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("I%d", rowNumber), fmt.Sprintf("=Round((F%d*G%d*H%d)/1000000,6)", rowNumber, rowNumber, rowNumber), styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("J%d", rowNumber), fmt.Sprintf("=Round(I%d*35.315,6)", rowNumber), styleFormula)

			err = addStringCellForSheet(f, sheetName, fmt.Sprintf("K%d", rowNumber), row.Country, style)
			err = addStringCellForSheet(f, sheetName, fmt.Sprintf("L%d", rowNumber), row.HsCode, style)
			err = addStringCellForSheet(f, sheetName, fmt.Sprintf("M%d", rowNumber), row.WebLink, style)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("N%d", rowNumber), 0.0, styleFormula)
			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("O%d", rowNumber), 0.0, styleFormula)
			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("P%d", rowNumber), row.Price, styleFormula)

			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("Q%d", rowNumber), fmt.Sprintf("=P%d", rowNumber), styleFormula)

			// marketplace
			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("R%d", rowNumber), row.EuVatRate, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("S%d", rowNumber), fmt.Sprintf("=Round(Q%d*(1-1/(1+R%d)), 6)", rowNumber, rowNumber), styleFormula)

			// platform cost
			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("T%d", rowNumber), row.ReferralFeeRate, styleFormula)

			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("U%d", rowNumber), fmt.Sprintf("=T%d", rowNumber), stylePercent)

			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("V%d", rowNumber), fmt.Sprintf("=Round(T%d*Q%d,6)", rowNumber, rowNumber), styleFormula)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("W%d", rowNumber), row.FulfilmentFee, styleFormula)
			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("X%d", rowNumber), row.StorageFeeRate, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("Y%d", rowNumber), fmt.Sprintf("=Round(X%d*I%d,6)", rowNumber, rowNumber), styleFormula)

			// local cost
			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("Z%d", rowNumber), row.GroundFeeRate, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AA%d", rowNumber), fmt.Sprintf("=Round(Z%d*E%d,6)", rowNumber, rowNumber), styleFormula)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AB%d", rowNumber), row.WarehouseFeeRate, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AC%d", rowNumber), fmt.Sprintf("=Round(AB%d*E%d,6)", rowNumber, rowNumber), styleFormula)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AD%d", rowNumber), row.ClearanceRate, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AE%d", rowNumber), fmt.Sprintf("=Round(AD%d*E%d,6)", rowNumber, rowNumber), styleFormula)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AF%d", rowNumber), row.DeliveryRate, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AG%d", rowNumber), fmt.Sprintf("=Round(AF%d*E%d,6)", rowNumber, rowNumber), styleFormula)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AH%d", rowNumber), row.WithinFeeRate, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AI%d", rowNumber), fmt.Sprintf("=Round(AH%d*E%d,6)", rowNumber, rowNumber), styleFormula)

			// subtotal
			subtotalFormula := fmt.Sprintf("=Round(AA%d+AC%d+AE%d+AG%d+AI%d,6)", rowNumber, rowNumber, rowNumber, rowNumber, rowNumber)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AJ%d", rowNumber), subtotalFormula, styleFormula)

			// customs value include duty
			customsValueIncludeDutyFormula := fmt.Sprintf("=Round(Q%d-(S%d+V%d+W%d+Y%d+AJ%d),6)",
				rowNumber, rowNumber, rowNumber, rowNumber, rowNumber, rowNumber)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AK%d", rowNumber), customsValueIncludeDutyFormula, styleFormula)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AM%d", rowNumber), row.EuDutyRate, styleFormula)

			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AL%d", rowNumber), fmt.Sprintf("=Round(AK%d/(1+AM%d),2)", rowNumber, rowNumber), styleFormula)

			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AN%d", rowNumber), fmt.Sprintf("=AM%d", rowNumber), stylePercent)

			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AO%d", rowNumber), fmt.Sprintf("=Round(AL%d*AM%d,2)", rowNumber, rowNumber), styleFormula)

			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AP%d", rowNumber), fmt.Sprintf("=Round(AL%d*D%d,2)", rowNumber, rowNumber), styleFormula)
			if err != nil {
				return err
			}
		}
	}

	// Save the spreadsheet with the origin path.
	if err = f.Save(); err != nil {
		return err
	}
	return nil
}

// fillBriefLwtExcel
func fillBriefLwtExcel(lwtFilePath string, rows []ExcelColumnForBriefLwt, sheetIdx int) error {
	f, err := excelize.OpenFile(lwtFilePath)
	if err != nil {
		fmt.Println(err)
	}

	defer func() {
		// Close the spreadsheet.
		if err := f.Close(); err != nil {
		}
	}()

	f.SetActiveSheet(sheetIdx)

	sheetName := f.GetSheetName(sheetIdx)

	fmt.Printf("sheetName: %s\n", sheetName)

	styleFormula, err := f.NewStyle(&excelize.Style{Border: border, Alignment: alignment, DecimalPlaces: FloatDecimalPlaces})
	style, err := f.NewStyle(&excelize.Style{Border: border, Alignment: alignment})

	if err != nil {
		fmt.Println("Create excel syle failed", err)
		log.Errorf("Create excel syle failed: %v", err)
		return err
	} else {
		fmt.Println("Begin to fill excel ...")
		for i := 0; i < len(rows); i++ {
			fmt.Println("Begin to fill excel, at:", i)
			rowNumber := InsertRowFirst + i

			err = f.InsertRow(sheetName, rowNumber)
			row := rows[i]

			err = addStringCellForSheet(f, sheetName, fmt.Sprintf("A%d", rowNumber), row.ItemNumber, style)
			billNo := ""
			if row.BillNo.Valid {
				billNo = row.BillNo.String
			}
			err = addStringCellForSheet(f, sheetName, fmt.Sprintf("B%d", rowNumber), billNo, style)

			platNo := ""
			if row.PlatoNo.Valid {
				platNo = row.PlatoNo.String
			}
			err = addStringCellForSheet(f, sheetName, fmt.Sprintf("C%d", rowNumber), platNo, style)

			trackingNo := ""
			// change: fill trackingNO is null
			//if row.TrackingNo.Valid {
			//	trackingNo = row.TrackingNo.String
			//}
			err = addStringCellForSheet(f, sheetName, fmt.Sprintf("D%d", rowNumber), trackingNo, style)

			err = addStringCellForSheet(f, sheetName, fmt.Sprintf("E%d", rowNumber), row.ProductNo, style)
			err = addStringCellForSheet(f, sheetName, fmt.Sprintf("F%d", rowNumber), row.Description, style)
			err = addStringCellForSheet(f, sheetName, fmt.Sprintf("G%d", rowNumber), row.Quantity, style)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("H%d", rowNumber), row.NetWeight, styleFormula)
			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("I%d", rowNumber), row.Height, styleFormula)
			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("J%d", rowNumber), row.Width, styleFormula)
			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("K%d", rowNumber), row.Length, styleFormula)

			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("L%d", rowNumber), fmt.Sprintf("=(I%d*J%d*K%d)/1000000", rowNumber, rowNumber, rowNumber), styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("M%d", rowNumber), fmt.Sprintf("=L%d*35.315", rowNumber), styleFormula)

			err = addStringCellForSheet(f, sheetName, fmt.Sprintf("N%d", rowNumber), row.Country, style)
			err = addStringCellForSheet(f, sheetName, fmt.Sprintf("O%d", rowNumber), row.HsCode, style)
			err = addStringCellForSheet(f, sheetName, fmt.Sprintf("P%d", rowNumber), row.WebLink, style)

			if err != nil {
				return err
			}
		}
	}

	// Save the spreadsheet with the origin path.
	if err = f.Save(); err != nil {
		return err
	}
	return nil
}

func addStringCellForSheet(f *excelize.File, sheetName string, cellName string, cellValue string, styleId int) error {
	err := f.SetCellStr(sheetName, cellName, cellValue)
	err = f.SetCellStyle(sheetName, cellName, cellName, styleId)
	if err != nil {
		return err
	}
	return nil
}

func addFloatCellForSheet(f *excelize.File, sheetName string, cellName string, cellValue float64, styleId int) error {
	err := f.SetCellFloat(sheetName, cellName, cellValue, FloatDecimalPlaces, 64)
	err = f.SetCellStyle(sheetName, cellName, cellName, styleId)
	if err != nil {
		return err
	}
	return nil
}

func addFormulaCellForSheet(f *excelize.File, sheetName string, cellName string, formula string, styleId int) error {
	err := f.SetCellFormula(sheetName, cellName, formula)
	err = f.SetCellStyle(sheetName, cellName, cellName, styleId)
	if err != nil {
		return err
	}
	return nil
}
