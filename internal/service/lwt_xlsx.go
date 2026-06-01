package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	log "github.com/sirupsen/logrus"
	"github.com/xuri/excelize/v2"
	"sysafari.com/customs/cguard/internal/config"
	"sysafari.com/customs/cguard/internal/database"
	"sysafari.com/customs/cguard/internal/model"
	"sysafari.com/customs/cguard/internal/script"
	"sysafari.com/customs/cguard/pkg/utils"
)

const (
	InsertRowFirst     = 4
	FloatDecimalPlaces = 6
	TimeLayout         = "20060102150405"
)

// LwtExcelSheetMap  SaleChannel 对应的sheet index
var LwtExcelSheetMap = map[string]int{
	"amazon":     0,
	"ebay":       1,
	"cdiscount":  2,
	"c_discount": 2,
}

// GenerateLWTExcel generate excel file for LWT
// incProfit 从 MQ 消息的 RequestForLwt.IncProfit 字段获取，如果消息中未传递此字段则默认为 false
func GenerateLWTExcel(data string) {
	response := &model.ResponseForLwt{
		Status:      "failed",
		LwtFilename: "",
		Error:       "",
	}
	requestForLwt, err := deserializeRequest(data)
	response.Brief = requestForLwt.Brief
	// 从 MQ 消息中获取 incProfit，如果未传递则默认为 false
	effectiveIncProfit := false
	if requestForLwt.IncProfit != nil {
		effectiveIncProfit = *requestForLwt.IncProfit
	}
	response.IncProfit = effectiveIncProfit

	if err != nil {
		response.Error = fmt.Sprintf("Deserialization of MQ message failed, err:%v", err)
	} else {
		var lwtFilename string
		if requestForLwt.Brief {
			lwtFilename, err = makeBriefLWT(requestForLwt.CustomsId)
		} else {
			lwtFilename, err = makeOfficialLWT(requestForLwt.CustomsId, effectiveIncProfit)
		}

		if err != nil {
			fmt.Println("Error, generate lwt file failed:", err)
			response.Error = fmt.Sprintf("Generate LWT excel failed,err:%v", err)
		} else {
			response.CustomsId = requestForLwt.CustomsId
			response.Status = "success"
			response.LwtFilename = lwtFilename
			response.Error = ""
			fmt.Println("Generate LWT excel success, lwtFilename:", lwtFilename)
		}

	}
	// pub to  rabbitmq
	fmt.Println("send lwt response to rabbitmq....")
	publishLwtResult(response)
}

// deserializeRequest is used to deserialize rabbitmq request
func deserializeRequest(message string) (model.RequestForLwt, error) {
	log.Infof("Deserialize request: %v", message)

	msg, err := strconv.Unquote(message)
	fmt.Println("msg:", msg)

	req := model.RequestForLwt{}
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

// publishLwtResult 发布LWT结果到RabbitMQ
func publishLwtResult(res *model.ResponseForLwt) {
	publishQueueName := config.GlobalConfig.RabbitMQ.Queue.LwtRes
	log.Infof("Lwt response: %v", res)

	marshal, err := json.Marshal(res)
	if err != nil {
		log.Errorf("Marshel struct to json failed: %v", err)
	} else {
		config.PublishMessage(string(marshal), publishQueueName)
	}
}

// isSplitCustoms 是否是拆单报关？@param customsId 主报关单号
func isSplitCustoms(customsId string) bool {
	var count int

	// 创建上下文
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 使用WithContext执行查询
	err := database.WithContext(ctx, func(db *sqlx.DB) error {
		return db.Get(&count, script.QueryCustomsSplitTotal, customsId)
	})

	if err != nil {
		log.Errorf("查询拆单总数失败: %v", err)
		return false
	}
	return count > 0
}

// makeOfficialLWTForNormal 普通的报关单生成LWT（未拆单报关）
func makeOfficialLWTForNormal(customsId string, incProfit bool) (string, error) {
	fmt.Println("1. query lwt data, customsId:", customsId)
	var rows []model.ExcelColumnForLwt
	err := database.GetDB().Select(&rows, script.QueryLwtData, customsId)
	if err != nil {
		return "", err
	}

	if len(rows) == 0 {
		return "", errors.New("cant not query rows for lwt")
	}

	return generateExcelForOfficialLWT(rows, incProfit)
}

// makeOfficialLWTForSplit 拆单报关单生成LWT
func makeOfficialLWTForSplit(customsId string, incProfit bool) (string, error) {
	// 1. 查询子报关单号
	fmt.Println("1. query customs split child, customsId:", customsId)
	var customsIds []string
	err := database.GetDB().Select(&customsIds, script.QueryCustomsSplitChild, customsId)
	if err != nil {
		return "", fmt.Errorf("query customs split child customsIds failed, err:%v", err)
	}
	// 2. 查询主单号的数据
	fmt.Println("2. query customs base info, customsId:", customsId)
	var customsBaseInfo model.CustomsBaseInfo
	err = database.GetDB().Get(&customsBaseInfo, script.QueryCustomsBaseInfo, customsId)
	if err != nil {
		return "", fmt.Errorf("query customs base info failed, err:%v", err)
	}

	// 3. 准备LWT文件模版及存放路径
	fmt.Println("3. ready for lwt file(template & save path), customsId:", customsId)
	fileSavePath, err := readyFowLwtFile(customsBaseInfo.DeclareCountry, customsId, "split", false, incProfit)
	if err != nil {
		return "", fmt.Errorf("ready file for lwt failed, err:%v", err)
	}

	// 4. 查询子单号的LWT 数据，并填充到LWT文件中
	// 有一个子单号的数据为空，就返回错误。不再继续执行
	fmt.Println("4. loop fill lwt excel for split, customsIds:", customsIds)
	for _, id := range customsIds {
		var rows []model.ExcelColumnForLwt
		err = database.GetDB().Select(&rows, script.QueryLwtDataForSplit, id)
		if err != nil || len(rows) == 0 {
			return "", err
		}

		salesChannel := strings.ToLower(rows[0].SalesChannel)
		idx, ok := LwtExcelSheetMap[salesChannel]
		if !ok {
			return "", fmt.Errorf("The map LwtExcelSheetMap dont have the sales channel %s.", salesChannel)
		}

		if customsBaseInfo.DeclareCountry == "BE" {
			if incProfit {
				err = fillLwtExcelForBeIncProfit(fileSavePath, rows, idx)
			} else {
				err = fillLwtExcelForBe(fileSavePath, rows, idx)
			}
		} else {
			if incProfit {
				err = fillLwtExcelForNlIncProfit(fileSavePath, rows, idx)
			} else {
				err = fillLwtExcelForNl(fileSavePath, rows, idx)
			}
		}
		if err != nil {
			return "", fmt.Errorf("fill lwt excel failed, err:%v", err)
		}
	}
	// 5. 返回文件名
	fmt.Println("5. return lwt file name: ", fileSavePath)
	return filepath.Base(fileSavePath), nil
}

// makeOfficialLWT Make official LWT Excel file
func makeOfficialLWT(customsId string, incProfit bool) (string, error) {
	// Is split into multiple sales channels?
	if isSplitCustoms(customsId) {
		fmt.Printf("-----LWT, customsId is split, customsId:%s -------\n", customsId)
		return makeOfficialLWTForSplit(customsId, incProfit)
	} else {
		fmt.Printf("-----LWT, customsId is normal, customsId:%s --------\n", customsId)
		return makeOfficialLWTForNormal(customsId, incProfit)
	}
}

// makeBriefLwtNormal 普通的简易报关文件LWT
func makeBriefLwtNormal(customsId string) (string, error) {
	fmt.Println("1. query brief lwt data, customsId:", customsId)
	var rows []model.ExcelColumnForBriefLwt
	err := database.GetDB().Select(&rows, script.QueryBriefLwtData, customsId)
	if err != nil {
		return "", fmt.Errorf("query brief lwt data failed, err:%v", err)
	}

	if len(rows) == 0 {
		return "", fmt.Errorf("cant not query rows for lwt")
	}

	fmt.Println("2. query plat and bill no, customsId:", customsId)
	var billPlat model.BillNoAndPlatForCustoms
	err = database.GetDB().Get(&billPlat, script.QueryPlatAndBillNo, customsId)
	if err != nil {
		return "", fmt.Errorf("query plat and bill no failed, err:%v", err)
	}

	// 填充plat和bill no
	fmt.Println("3. fill plat and bill no, customsId:", customsId)
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
	err := database.GetDB().Select(&customsIds, script.QueryCustomsSplitChild, customsId)
	if err != nil {
		log.Errorf("query customs split child customsIds failed, err:%v, sql:%s", err, script.QueryCustomsSplitChild)
		return "", fmt.Errorf("query customs split child customsIds failed, err:%v", err)
	}
	// 2. 查询主单号的数据
	fmt.Println("2. query customs base info, customsId:", customsId)
	var customsBaseInfo model.CustomsBaseInfo
	err = database.GetDB().Get(&customsBaseInfo, script.QueryCustomsBaseInfo, customsId)
	if err != nil {
		return "", fmt.Errorf("query customs base info failed, err:%v", err)
	}
	// 3. 查询主单号的plat和bill no
	fmt.Println("3. query plat and bill no, customsId:", customsId)
	var billPlat model.BillNoAndPlatForCustoms
	err = database.GetDB().Get(&billPlat, script.QueryPlatAndBillNo, customsId)
	if err != nil {
		return "", fmt.Errorf("query plat and bill no failed, err:%v", err)
	}

	// 4. 准备LWT文件模版及存放路径
	fmt.Println("4. ready for lwt file(template & save path), customsId:", customsId)
	fileSavePath, err := readyFowLwtFile(customsBaseInfo.DeclareCountry, customsId, "split", true, false)
	if err != nil {
		return "", fmt.Errorf("ready file for lwt failed, err:%v", err)
	}

	// 4. 查询子单号的LWT 数据，并填充到LWT文件中
	// 有一个子单号的数据为空，就返回错误。不再继续执行
	fmt.Println("5. loop fill brief lwt excel for split, customsIds:", customsIds)
	for _, id := range customsIds {
		var rows []model.ExcelColumnForBriefLwt
		err = database.GetDB().Select(&rows, script.QueryBriefLwtDataForSplit, id)
		if err != nil || len(rows) == 0 {
			return "", fmt.Errorf("query brief lwt data for split failed, err:%v", err)
		}

		salesChannel := strings.ToLower(rows[0].SalesChannel)
		idx, ok := LwtExcelSheetMap[salesChannel]
		if !ok {
			return "", fmt.Errorf("The map LwtExcelSheetMap dont have the sales channel %s.", salesChannel)
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
			return "", fmt.Errorf("fill brief lwt excel failed, err:%v", err)
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
		fmt.Printf("-----Brief LWT, customsId is split, customsId:%s -------\n", customsId)
		return makeBriefLwtForSplit(customsId)
	} else {
		fmt.Printf("-----Brief LWT, customsId is normal, customsId:%s --------\n", customsId)
		return makeBriefLwtNormal(customsId)
	}
}

// GenerateLWTExcel generate excel file for LWT,
// error =nil returns lwt file link(oss)
func generateExcelForOfficialLWT(rows []model.ExcelColumnForLwt, incProfit bool) (string, error) {
	declareCountry := rows[0].DeclareCountry
	customId := rows[0].CustomsId
	salesChannel := rows[0].SalesChannel

	fmt.Println("2. ready for lwt file(template & save path), declareCountry:", declareCountry, "customId:", customId, "salesChannel:", salesChannel, "incProfit:", incProfit)

	// 注意：当前仅ebay 支持利润率计算,不区分大小写
	if strings.ToLower(salesChannel) != "ebay" {
		fmt.Println("**** DEBUG: salesChannel not ebay, incProfit set to: false")
		incProfit = false
	}

	lwtFilePath, err := readyFowLwtFile(declareCountry, customId, salesChannel, false, incProfit)
	fmt.Println("lwtFilePath: ", lwtFilePath)
	if err != nil {
		return "", fmt.Errorf("Prepare LWT file failed, err:%v", err)
	}

	fmt.Println("3. fill lwt excel, declareCountry:", declareCountry)
	if "BE" == strings.ToUpper(declareCountry) {
		if incProfit {
			err = fillLwtExcelForBeIncProfit(lwtFilePath, rows, 0)
		} else {
			err = fillLwtExcelForBe(lwtFilePath, rows, 0)
		}
	} else {
		if incProfit {
			err = fillLwtExcelForNlIncProfit(lwtFilePath, rows, 0)
		} else {
			err = fillLwtExcelForNl(lwtFilePath, rows, 0)
		}
	}

	if err != nil {
		return "", fmt.Errorf("Fill LWT excel failed, err:%v", err)
	}

	fmt.Println("4. return lwt file name: ", lwtFilePath)

	return filepath.Base(lwtFilePath), nil
}

// GenerateLWTExcel generate excel file for LWT,
// error =nil returns lwt file link(oss)
func generateExcelForBriefLWT(rows []model.ExcelColumnForBriefLwt) (string, error) {
	declareCountry := rows[0].DeclareCountry
	customId := rows[0].CustomsId
	salesChannel := rows[0].SalesChannel

	fmt.Println("4. ready for lwt file(template & save path), declareCountry:", declareCountry, "customId:", customId, "salesChannel:", salesChannel)
	lwtFilePath, err := readyFowLwtFile(declareCountry, customId, salesChannel, true, false)
	if err != nil {
		return "", err
	}

	fmt.Println("5. fill brief lwt excel...")
	err = fillBriefLwtExcel(lwtFilePath, rows, 0)
	if err != nil {
		return "", err
	}

	fmt.Println("6. return brief lwt file name: ", lwtFilePath)
	return filepath.Base(lwtFilePath), nil
}

// readyFowLwtFile 准备LWT文件模版和存放路径。如果是拆分报关，salesChannel为split
// incProfit: true表示计算利润率，false表示不计算利润率,默认不计算利润率(表示当前最新模版文件)
func readyFowLwtFile(declareCountry, customId, salesChannel string, brief bool, incProfit bool) (string, error) {
	cfg := config.GetConfig()
	var templatePath string

	// 添加调试日志：打印所有参数值
	fmt.Printf("**** DEBUG readyFowLwtFile: declareCountry=%s, customId=%s, salesChannel=%s, brief=%v, incProfit=%v\n",
		declareCountry, customId, salesChannel, brief, incProfit)

	salesChannel = strings.ToLower(salesChannel)
	salesChannel = strings.ToLower(salesChannel)
	declareCountry = strings.ToLower(declareCountry)

	// 添加调试日志：打印转换后的值
	fmt.Printf("**** DEBUG after toLower: declareCountry=%s, salesChannel=%s\n", declareCountry, salesChannel)

	if brief {
		fmt.Println("**** DEBUG: 进入 brief 分支")
		switch salesChannel {
		case "amazon":
			templatePath = cfg.LWT.Template.Brief.Amazon
		case "ebay":
			templatePath = cfg.LWT.Template.Brief.Ebay
		case "cdiscount", "c_discount":
			templatePath = cfg.LWT.Template.Brief.CDiscount
		case "split":
			templatePath = cfg.LWT.Template.Brief.Split
		default:
			return "", fmt.Errorf("SalesChannel: %s not supported for BRIEF LWT.", salesChannel)
		}
	} else {
		fmt.Println("**** DEBUG: 进入 else 分支 (非 brief)")
		switch declareCountry {
		case "nl":
			fmt.Println("**** DEBUG: declareCountry 是 nl")
			switch salesChannel {
			case "amazon":
				templatePath = cfg.LWT.Template.Official.NL.Amazon
			case "ebay":
				if incProfit {
					templatePath = cfg.LWT.Template.Profit.NL.Ebay
				} else {
					templatePath = cfg.LWT.Template.Official.NL.Ebay
				}
			case "cdiscount", "c_discount":
				templatePath = cfg.LWT.Template.Official.NL.CDiscount
			case "split":
				templatePath = cfg.LWT.Template.Official.NL.Split
			default:
				return "", fmt.Errorf("SalesChannel: %s not supported for LWT.", salesChannel)
			}
		case "be":
			fmt.Println("**** DEBUG: declareCountry 是 be")
			switch salesChannel {
			case "amazon":
				templatePath = cfg.LWT.Template.Official.BE.Amazon
			case "ebay":
				fmt.Printf("**** DEBUG: salesChannel 是 ebay, incProfit=%v\n", incProfit)
				if incProfit {
					fmt.Println("**** profit.be.ebay: ", cfg.LWT.Template.Profit.BE.Ebay)
					templatePath = cfg.LWT.Template.Profit.BE.Ebay
				} else {
					templatePath = cfg.LWT.Template.Official.BE.Ebay
				}
			case "cdiscount", "c_discount":
				templatePath = cfg.LWT.Template.Official.BE.CDiscount
			case "split":
				templatePath = cfg.LWT.Template.Official.BE.Split
			default:
				fmt.Printf("**** DEBUG: salesChannel=%s 不匹配任何 case，进入 default\n", salesChannel)
				return "", fmt.Errorf("SalesChannel: %s not supported for LWT.", salesChannel)
			}
		default:
			fmt.Printf("**** DEBUG: declareCountry=%s 不匹配任何 case，进入 default\n", declareCountry)
			return "", fmt.Errorf("Country: %s not supported for LWT.", declareCountry)
		}
	}

	fmt.Println("**** templatePath: ", templatePath)
	if templatePath == "" {
		return "", fmt.Errorf("SalesChannel: %s not supports to LWT.", salesChannel)
	}

	if !utils.IsExists(templatePath) {
		return "", fmt.Errorf("Template file: %s does not exist", templatePath)
	}

	tmpDir := cfg.LWT.Tmp.Dir
	if !utils.IsDir(tmpDir) && !utils.CreateDir(tmpDir) {
		return "", fmt.Errorf("Crate tmp directory: %s failed !", tmpDir)
	}

	now := time.Now()
	timestamp := now.Format(TimeLayout)
	saveDir := filepath.Join(tmpDir, strconv.Itoa(now.Year()), strconv.Itoa(int(now.Month())))
	if !utils.IsDir(saveDir) && !utils.CreateDir(saveDir) {
		return "", fmt.Errorf("Create save dir: %s failed !", saveDir)
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
		return "", fmt.Errorf("Create lwt file: %s form template file: %s failed!", lwtFilePath, templatePath)
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

// fillLwtExcelForNl fill data to lwt excel file for NL(include profit rate calculation and all ecp fees calculation)
func fillLwtExcelForNlIncProfit(lwtFilePath string, rows []model.ExcelColumnForLwt, sheetIdx int) error {
	f, err := excelize.OpenFile(lwtFilePath)
	if err != nil {
		fmt.Println("fill lwt excel file for nl include profit,open file failed", err)
	}

	defer func() {
		// Close the spreadsheet.
		if err := f.Close(); err != nil {
		}
	}()

	f.SetActiveSheet(sheetIdx)

	sheetName := f.GetSheetName(sheetIdx)

	fmt.Printf("sheetName: %s\n", sheetName)

	decimalPlaces := FloatDecimalPlaces
	styleFormula, err := f.NewStyle(&excelize.Style{Border: border, Alignment: alignment, DecimalPlaces: &decimalPlaces})
	style, err := f.NewStyle(&excelize.Style{Border: border, Alignment: alignment})
	stylePercent, err := f.NewStyle(&excelize.Style{Border: border, Alignment: alignment, NumFmt: 10, Font: font})

	if err != nil {
		log.Errorf("Create excel syle failed: %v", err)
	} else {
		for i := 0; i < len(rows); i++ {
			rowNumber := InsertRowFirst + i

			err = f.InsertRows(sheetName, rowNumber, 1)
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

			// platform cost(ecp fees)
			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("T%d", rowNumber), row.ReferralFeeRate, styleFormula)

			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("U%d", rowNumber), fmt.Sprintf("=T%d", rowNumber), stylePercent)

			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("V%d", rowNumber), fmt.Sprintf("=Round(T%d*Q%d,6)", rowNumber, rowNumber), styleFormula)
			// 原计算模版 多出的费用
			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("W%d", rowNumber), row.ClosingFee.Float64, styleFormula)
			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("X%d", rowNumber), row.HighVolumeListingFee.Float64, styleFormula)
			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("Y%d", rowNumber), row.ProcessingFeeRate, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("Z%d", rowNumber), fmt.Sprintf("=Round(Y%d*Q%d,6)", rowNumber, rowNumber), styleFormula)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AA%d", rowNumber), row.AuthorisationFee.Float64, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AB%d", rowNumber), fmt.Sprintf("=AA%d", rowNumber), styleFormula)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AC%d", rowNumber), row.InterchangeableFeeRate, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AD%d", rowNumber), fmt.Sprintf("=Round(AC%d*Q%d,6)", rowNumber, rowNumber), styleFormula)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AE%d", rowNumber), row.FulfilmentFee.Float64, styleFormula)
			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AF%d", rowNumber), row.StorageFeeRate, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AG%d", rowNumber), fmt.Sprintf("=Round(AF%d*I%d,6)", rowNumber, rowNumber), styleFormula)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AH%d", rowNumber), row.AdvertisingFee.Float64, styleFormula)

			// profit
			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AI%d", rowNumber), row.ProfitRate, styleFormula)
			profitFormula := fmt.Sprintf("=Round(AI%d*Q%d,6)",rowNumber, rowNumber)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AJ%d", rowNumber), profitFormula, styleFormula)

			// local cost
			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AK%d", rowNumber), row.GroundFeeRate, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AL%d", rowNumber), fmt.Sprintf("=Round(AK%d*E%d,6)", rowNumber, rowNumber), styleFormula)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AM%d", rowNumber), row.WarehouseFeeRate, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AN%d", rowNumber), fmt.Sprintf("=Round(AM%d*E%d,6)", rowNumber, rowNumber), styleFormula)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AO%d", rowNumber), row.ClearanceRate, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AP%d", rowNumber), fmt.Sprintf("=Round(AO%d*E%d,6)", rowNumber, rowNumber), styleFormula)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AQ%d", rowNumber), row.DeliveryRate, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AR%d", rowNumber), fmt.Sprintf("=Round(AQ%d*E%d,6)", rowNumber, rowNumber), styleFormula)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AS%d", rowNumber), row.WithinFeeRate, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AT%d", rowNumber), fmt.Sprintf("=Round(AS%d*E%d,6)", rowNumber, rowNumber), styleFormula)

			// subtotal
			subtotalFormula := fmt.Sprintf("=Round(AL%d+AN%d+AP%d+AR%d+AT%d,6)", rowNumber, rowNumber, rowNumber, rowNumber, rowNumber)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AU%d", rowNumber), subtotalFormula, styleFormula)

			// customs value include duty
			customsValueIncludeDutyFormula := fmt.Sprintf("=Round(Q%d-(S%d+V%d+W%d+X%d+Z%d+AB%d+AD%d+AE%d+AG%d+AH%d+AJ%d+AU%d),2)",
				rowNumber, rowNumber, rowNumber, rowNumber, rowNumber, rowNumber, rowNumber, rowNumber, rowNumber, rowNumber, rowNumber, rowNumber, rowNumber)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AV%d", rowNumber), customsValueIncludeDutyFormula, styleFormula)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AX%d", rowNumber), row.EuDutyRate, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AY%d", rowNumber), fmt.Sprintf("=AX%d", rowNumber), stylePercent)

			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AW%d", rowNumber), fmt.Sprintf("=Round(AV%d/(1+AX%d),2)", rowNumber, rowNumber), styleFormula)

			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AZ%d", rowNumber), fmt.Sprintf("=Round(AW%d*AX%d,2)", rowNumber, rowNumber), styleFormula)

			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("BA%d", rowNumber), fmt.Sprintf("=Round(AW%d*D%d,2)", rowNumber, rowNumber), styleFormula)
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

// fillLwtExcelForNl fill data to lwt excel file for NL
func fillLwtExcelForNl(lwtFilePath string, rows []model.ExcelColumnForLwt, sheetIdx int) error {
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

	decimalPlaces := FloatDecimalPlaces
	styleFormula, err := f.NewStyle(&excelize.Style{Border: border, Alignment: alignment, DecimalPlaces: &decimalPlaces})
	style, err := f.NewStyle(&excelize.Style{Border: border, Alignment: alignment})
	stylePercent, err := f.NewStyle(&excelize.Style{Border: border, Alignment: alignment, NumFmt: 10, Font: font})

	if err != nil {
		log.Errorf("Create excel syle failed: %v", err)
	} else {
		for i := 0; i < len(rows); i++ {
			rowNumber := InsertRowFirst + i

			err = f.InsertRows(sheetName, rowNumber, 1)
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

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("Y%d", rowNumber), row.AuthorisationFee.Float64, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("Z%d", rowNumber), fmt.Sprintf("=Y%d", rowNumber), styleFormula)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AA%d", rowNumber), row.InterchangeableFeeRate, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AB%d", rowNumber), fmt.Sprintf("=Round(AA%d*Q%d,6)", rowNumber, rowNumber), styleFormula)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AC%d", rowNumber), row.FulfilmentFee.Float64, styleFormula)
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
			customsValueIncludeDutyFormula := fmt.Sprintf("=Round(Q%d-(S%d+V%d+X%d+Z%d+AB%d+AC%d+AE%d+AP%d),2)",
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

// fillLwtExcelForBeIncProfit fill data to lwt excel file for BE(include profit rate calculation and all ecp fees calculation)
func fillLwtExcelForBeIncProfit(lwtFilePath string, rows []model.ExcelColumnForLwt, sheetIdx int) error {
	f, err := excelize.OpenFile(lwtFilePath)
	if err != nil {
		fmt.Println("fill lwt excel file for be include profit,open file failed", err)
	}

	defer func() {
		// Close the spreadsheet.
		if err := f.Close(); err != nil {
		}
	}()

	f.SetActiveSheet(sheetIdx)

	sheetName := f.GetSheetName(sheetIdx)

	fmt.Printf("sheetName: %s\n", sheetName)

	decimalPlaces := FloatDecimalPlaces
	styleFormula, err := f.NewStyle(&excelize.Style{Border: border, Alignment: alignment, DecimalPlaces: &decimalPlaces})
	style, err := f.NewStyle(&excelize.Style{Border: border, Alignment: alignment})
	stylePercent, err := f.NewStyle(&excelize.Style{Border: border, Alignment: alignment, NumFmt: 10, Font: font})

	if err != nil {
		log.Errorf("Create excel syle failed: %v", err)
	} else {
		for i := 0; i < len(rows); i++ {
			rowNumber := InsertRowFirst + i

			err = f.InsertRows(sheetName, rowNumber, 1)
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

			// platform cost(ecp fees)
			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("T%d", rowNumber), row.ReferralFeeRate, styleFormula)

			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("U%d", rowNumber), fmt.Sprintf("=T%d", rowNumber), stylePercent)

			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("V%d", rowNumber), fmt.Sprintf("=Round(T%d*Q%d,6)", rowNumber, rowNumber), styleFormula)
			// 原计算模版 多出的费用
			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("W%d", rowNumber), row.ClosingFee.Float64, styleFormula)
			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("X%d", rowNumber), row.HighVolumeListingFee.Float64, styleFormula)
			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("Y%d", rowNumber), row.ProcessingFeeRate, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("Z%d", rowNumber), fmt.Sprintf("=Round(Y%d*Q%d,6)", rowNumber, rowNumber), styleFormula)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AA%d", rowNumber), row.AuthorisationFee.Float64, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AB%d", rowNumber), fmt.Sprintf("=AA%d", rowNumber), styleFormula)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AC%d", rowNumber), row.InterchangeableFeeRate, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AD%d", rowNumber), fmt.Sprintf("=Round(AC%d*Q%d,6)", rowNumber, rowNumber), styleFormula)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AE%d", rowNumber), row.FulfilmentFee.Float64, styleFormula)
			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AF%d", rowNumber), row.StorageFeeRate, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AG%d", rowNumber), fmt.Sprintf("=Round(AF%d*I%d,6)", rowNumber, rowNumber), styleFormula)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AH%d", rowNumber), row.AdvertisingFee.Float64, styleFormula)

			// profit
			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AI%d", rowNumber), row.ProfitRate, styleFormula)
			profitFormula := fmt.Sprintf("=Round(AI%d*Q%d,6)",rowNumber, rowNumber)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AJ%d", rowNumber), profitFormula, styleFormula)

			// local cost
			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AK%d", rowNumber), row.GroundFeeRate, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AL%d", rowNumber), fmt.Sprintf("=Round(AK%d*E%d,6)", rowNumber, rowNumber), styleFormula)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AM%d", rowNumber), row.WarehouseFeeRate, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AN%d", rowNumber), fmt.Sprintf("=Round(AM%d*E%d,6)", rowNumber, rowNumber), styleFormula)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AO%d", rowNumber), row.ClearanceRate, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AP%d", rowNumber), fmt.Sprintf("=Round(AO%d*E%d,6)", rowNumber, rowNumber), styleFormula)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AQ%d", rowNumber), row.DeliveryRate, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AR%d", rowNumber), fmt.Sprintf("=Round(AQ%d*E%d,6)", rowNumber, rowNumber), styleFormula)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AS%d", rowNumber), row.WithinFeeRate, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AT%d", rowNumber), fmt.Sprintf("=Round(AS%d*E%d,6)", rowNumber, rowNumber), styleFormula)

			// subtotal
			subtotalFormula := fmt.Sprintf("=Round(AL%d+AN%d+AP%d+AR%d+AT%d,6)", rowNumber, rowNumber, rowNumber, rowNumber, rowNumber)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AU%d", rowNumber), subtotalFormula, styleFormula)

			// customs value include duty
			customsValueIncludeDutyFormula := fmt.Sprintf("=Round(Q%d-(S%d+V%d+W%d+X%d+Z%d+AB%d+AD%d+AE%d+AG%d+AH%d+AJ%d+AU%d),2)",
				rowNumber, rowNumber, rowNumber, rowNumber, rowNumber, rowNumber, rowNumber, rowNumber, rowNumber, rowNumber, rowNumber, rowNumber, rowNumber)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AV%d", rowNumber), customsValueIncludeDutyFormula, styleFormula)

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("AX%d", rowNumber), row.EuDutyRate, styleFormula)
			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AY%d", rowNumber), fmt.Sprintf("=AX%d", rowNumber), stylePercent)

			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AW%d", rowNumber), fmt.Sprintf("=Round(AV%d/(1+AX%d),2)", rowNumber, rowNumber), styleFormula)

			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("AZ%d", rowNumber), fmt.Sprintf("=Round(AW%d*AX%d,2)", rowNumber, rowNumber), styleFormula)

			err = addFormulaCellForSheet(f, sheetName, fmt.Sprintf("BA%d", rowNumber), fmt.Sprintf("=Round(AW%d*D%d,2)", rowNumber, rowNumber), styleFormula)
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
func fillLwtExcelForBe(lwtFilePath string, rows []model.ExcelColumnForLwt, sheetIdx int) error {
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

	decimalPlaces := FloatDecimalPlaces
	styleFormula, err := f.NewStyle(&excelize.Style{Border: border, Alignment: alignment, DecimalPlaces: &decimalPlaces})
	style, err := f.NewStyle(&excelize.Style{Border: border, Alignment: alignment})
	stylePercent, err := f.NewStyle(&excelize.Style{Border: border, Alignment: alignment, NumFmt: 10, Font: font})

	if err != nil {
		log.Errorf("Create excel syle failed: %v", err)
	} else {
		for i := 0; i < len(rows); i++ {
			rowNumber := InsertRowFirst + i

			err = f.InsertRows(sheetName, rowNumber, 1)
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

			err = addFloatCellForSheet(f, sheetName, fmt.Sprintf("W%d", rowNumber), row.FulfilmentFee.Float64, styleFormula)
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
func fillBriefLwtExcel(lwtFilePath string, rows []model.ExcelColumnForBriefLwt, sheetIdx int) error {
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

	decimalPlaces := FloatDecimalPlaces
	styleFormula, err := f.NewStyle(&excelize.Style{Border: border, Alignment: alignment, DecimalPlaces: &decimalPlaces})
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

			err = f.InsertRows(sheetName, rowNumber, 1)
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
