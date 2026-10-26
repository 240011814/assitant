package service

import (
	"backend/model"
	"encoding/json"
	"fmt"
	"log"
	"strconv"

	"gorm.io/gorm/clause"
)

// 宏观经济数据同步 (复用 StockSyncService 的串行 HTTP client 与防重入状态)
// 数据量小(利率/准备金率约几十条, 货币供应量数百条), 每次全量拉取后 upsert 幂等写入
// baostock 每日调用限额, 五个接口各占 1 次调用

// SyncMacroData 同步宏观经济数据: 存款利率/贷款利率/存款准备金率/货币供应量(月度/年度)/LPR/GDP/CPI/PMI/PPI
func (s *StockSyncService) SyncMacroData(force bool) error {
	log.Printf("[StockSync] 同步宏观经济数据... (force=%v)", force)
	if err := s.syncDepositRate(); err != nil {
		return err
	}
	if err := s.syncLoanRate(); err != nil {
		return err
	}
	if err := s.syncReserveRatio(); err != nil {
		return err
	}
	if err := s.syncMoneySupplyMonth(); err != nil {
		return err
	}
	if err := s.syncMoneySupplyYear(); err != nil {
		return err
	}
	if err := s.syncLPR(); err != nil {
		return err
	}
	if err := s.syncGDP(); err != nil {
		return err
	}
	if err := s.syncCPI(); err != nil {
		return err
	}
	if err := s.syncPMI(); err != nil {
		return err
	}
	if err := s.syncPPI(); err != nil {
		return err
	}
	log.Printf("[StockSync] 同步宏观经济数据完成")
	return nil
}

// syncDepositRate 同步存款利率 (query_deposit_rate_data, 1次调用全量, 无参数返回1990年起全部历史)
func (s *StockSyncService) syncDepositRate() error {
	url := fmt.Sprintf("%s/query_deposit_rate_data", s.baostockURL)
	body, err := s.httpGetWithDelay(url)
	if err != nil {
		return fmt.Errorf("获取存款利率失败: %v", err)
	}

	var resp struct {
		Ok   bool `json:"ok"`
		Data struct {
			Items []struct {
				PubDate                          string `json:"pubDate"`
				DemandDepositRate                string `json:"demandDepositRate"`
				FixedDepositRate3Month           string `json:"fixedDepositRate3Month"`
				FixedDepositRate6Month           string `json:"fixedDepositRate6Month"`
				FixedDepositRate1Year            string `json:"fixedDepositRate1Year"`
				FixedDepositRate2Year            string `json:"fixedDepositRate2Year"`
				FixedDepositRate3Year            string `json:"fixedDepositRate3Year"`
				FixedDepositRate5Year            string `json:"fixedDepositRate5Year"`
				InstallmentFixedDepositRate1Year string `json:"installmentFixedDepositRate1Year"`
				InstallmentFixedDepositRate3Year string `json:"installmentFixedDepositRate3Year"`
				InstallmentFixedDepositRate5Year string `json:"installmentFixedDepositRate5Year"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("解析存款利率失败: %v", err)
	}
	if !resp.Ok {
		return fmt.Errorf("获取存款利率失败")
	}

	rows := make([]model.MacroDepositRate, 0, len(resp.Data.Items))
	for _, item := range resp.Data.Items {
		pubDate, err := parseTradeDay(item.PubDate)
		if err != nil {
			continue
		}
		rows = append(rows, model.MacroDepositRate{
			PubDate:       pubDate,
			Demand:        parseFloatPtr(item.DemandDepositRate),
			Fixed3Month:   parseFloatPtr(item.FixedDepositRate3Month),
			Fixed6Month:   parseFloatPtr(item.FixedDepositRate6Month),
			Fixed1Year:    parseFloatPtr(item.FixedDepositRate1Year),
			Fixed2Year:    parseFloatPtr(item.FixedDepositRate2Year),
			Fixed3Year:    parseFloatPtr(item.FixedDepositRate3Year),
			Fixed5Year:    parseFloatPtr(item.FixedDepositRate5Year),
			Installment1Y: parseFloatPtr(item.InstallmentFixedDepositRate1Year),
			Installment3Y: parseFloatPtr(item.InstallmentFixedDepositRate3Year),
			Installment5Y: parseFloatPtr(item.InstallmentFixedDepositRate5Year),
		})
	}
	if len(rows) == 0 {
		log.Printf("[StockSync] 存款利率无数据")
		return nil
	}

	if err := DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "pub_date"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"demand", "fixed_3m", "fixed_6m", "fixed_1y", "fixed_2y", "fixed_3y", "fixed_5y",
			"installment_1y", "installment_3y", "installment_5y",
		}),
	}).Create(&rows).Error; err != nil {
		return fmt.Errorf("写入存款利率失败: %v", err)
	}
	log.Printf("[StockSync] 存款利率同步: %d 条", len(rows))
	return nil
}

// syncLoanRate 同步贷款利率 (query_loan_rate_data, 1次调用全量, 无参数返回1990年起全部历史)
func (s *StockSyncService) syncLoanRate() error {
	url := fmt.Sprintf("%s/query_loan_rate_data", s.baostockURL)
	body, err := s.httpGetWithDelay(url)
	if err != nil {
		return fmt.Errorf("获取贷款利率失败: %v", err)
	}

	var resp struct {
		Ok   bool `json:"ok"`
		Data struct {
			Items []struct {
				PubDate                string `json:"pubDate"`
				LoanRate6Month         string `json:"loanRate6Month"`
				LoanRate6MonthTo1Year  string `json:"loanRate6MonthTo1Year"`
				LoanRate1YearTo3Year   string `json:"loanRate1YearTo3Year"`
				LoanRate3YearTo5Year   string `json:"loanRate3YearTo5Year"`
				LoanRateAbove5Year     string `json:"loanRateAbove5Year"`
				MortgateRateBelow5Year string `json:"mortgateRateBelow5Year"`
				MortgateRateAbove5Year string `json:"mortgateRateAbove5Year"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("解析贷款利率失败: %v", err)
	}
	if !resp.Ok {
		return fmt.Errorf("获取贷款利率失败")
	}

	rows := make([]model.MacroLoanRate, 0, len(resp.Data.Items))
	for _, item := range resp.Data.Items {
		pubDate, err := parseTradeDay(item.PubDate)
		if err != nil {
			continue
		}
		rows = append(rows, model.MacroLoanRate{
			PubDate:         pubDate,
			Loan6Month:      parseFloatPtr(item.LoanRate6Month),
			Loan6MonthTo1Y:  parseFloatPtr(item.LoanRate6MonthTo1Year),
			Loan1YTo3Y:      parseFloatPtr(item.LoanRate1YearTo3Year),
			Loan3YTo5Y:      parseFloatPtr(item.LoanRate3YearTo5Year),
			LoanAbove5Y:     parseFloatPtr(item.LoanRateAbove5Year),
			MortgageBelow5Y: parseFloatPtr(item.MortgateRateBelow5Year),
			MortgageAbove5Y: parseFloatPtr(item.MortgateRateAbove5Year),
		})
	}
	if len(rows) == 0 {
		log.Printf("[StockSync] 贷款利率无数据")
		return nil
	}

	if err := DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "pub_date"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"loan_6m", "loan_6m_1y", "loan_1y_3y", "loan_3y_5y", "loan_above_5y",
			"mortgage_below_5y", "mortgage_above_5y",
		}),
	}).Create(&rows).Error; err != nil {
		return fmt.Errorf("写入贷款利率失败: %v", err)
	}
	log.Printf("[StockSync] 贷款利率同步: %d 条", len(rows))
	return nil
}

// syncReserveRatio 同步存款准备金率 (query_required_reserve_ratio_data, 1次调用全量)
// 接口日期过滤存在异常(指定年份区间返回0条), 因此不传日期, 全量拉取后 upsert
func (s *StockSyncService) syncReserveRatio() error {
	url := fmt.Sprintf("%s/query_required_reserve_ratio_data", s.baostockURL)
	body, err := s.httpGetWithDelay(url)
	if err != nil {
		return fmt.Errorf("获取存款准备金率失败: %v", err)
	}

	var resp struct {
		Ok   bool `json:"ok"`
		Data struct {
			Items []struct {
				PubDate                      string `json:"pubDate"`
				EffectiveDate                string `json:"effectiveDate"`
				BigInstitutionsRatioPre      string `json:"bigInstitutionsRatioPre"`
				BigInstitutionsRatioAfter    string `json:"bigInstitutionsRatioAfter"`
				MediumInstitutionsRatioPre   string `json:"mediumInstitutionsRatioPre"`
				MediumInstitutionsRatioAfter string `json:"mediumInstitutionsRatioAfter"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("解析存款准备金率失败: %v", err)
	}
	if !resp.Ok {
		return fmt.Errorf("获取存款准备金率失败")
	}

	rows := make([]model.MacroReserveRatio, 0, len(resp.Data.Items))
	for _, item := range resp.Data.Items {
		pubDate, err := parseTradeDay(item.PubDate)
		if err != nil {
			continue
		}
		effectiveDate, err := parseTradeDay(item.EffectiveDate)
		if err != nil {
			continue
		}
		rows = append(rows, model.MacroReserveRatio{
			PubDate:                      pubDate,
			EffectiveDate:                effectiveDate,
			BigInstitutionsRatioPre:      parseFloatPtr(item.BigInstitutionsRatioPre),
			BigInstitutionsRatioAfter:    parseFloatPtr(item.BigInstitutionsRatioAfter),
			MediumInstitutionsRatioPre:   parseFloatPtr(item.MediumInstitutionsRatioPre),
			MediumInstitutionsRatioAfter: parseFloatPtr(item.MediumInstitutionsRatioAfter),
		})
	}
	if len(rows) == 0 {
		log.Printf("[StockSync] 存款准备金率无数据")
		return nil
	}

	const batchSize = 500
	for i := 0; i < len(rows); i += batchSize {
		end := min(i+batchSize, len(rows))
		batch := rows[i:end]
		if err := DB.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "pub_date"}, {Name: "effective_date"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"big_institutions_ratio_pre", "big_institutions_ratio_after",
				"medium_institutions_ratio_pre", "medium_institutions_ratio_after",
			}),
		}).Create(&batch).Error; err != nil {
			return fmt.Errorf("写入存款准备金率失败: %v", err)
		}
	}
	log.Printf("[StockSync] 存款准备金率同步: %d 条", len(rows))
	return nil
}

// syncMoneySupplyMonth 同步货币供应量月度数据 (query_money_supply_data_month, 1次调用全量)
func (s *StockSyncService) syncMoneySupplyMonth() error {
	url := fmt.Sprintf("%s/query_money_supply_data_month", s.baostockURL)
	body, err := s.httpGetWithDelay(url)
	if err != nil {
		return fmt.Errorf("获取货币供应量(月度)失败: %v", err)
	}

	var resp struct {
		Ok   bool `json:"ok"`
		Data struct {
			Items []struct {
				StatYear        string `json:"statYear"`
				StatMonth       string `json:"statMonth"`
				M0Month         string `json:"m0Month"`
				M0YOY           string `json:"m0YOY"`
				M0ChainRelative string `json:"m0ChainRelative"`
				M1Month         string `json:"m1Month"`
				M1YOY           string `json:"m1YOY"`
				M1ChainRelative string `json:"m1ChainRelative"`
				M2Month         string `json:"m2Month"`
				M2YOY           string `json:"m2YOY"`
				M2ChainRelative string `json:"m2ChainRelative"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("解析货币供应量(月度)失败: %v", err)
	}
	if !resp.Ok {
		return fmt.Errorf("获取货币供应量(月度)失败")
	}

	rows := make([]model.MacroMoneySupplyMonth, 0, len(resp.Data.Items))
	for _, item := range resp.Data.Items {
		year, err := strconv.Atoi(item.StatYear)
		if err != nil {
			continue
		}
		month, err := strconv.Atoi(item.StatMonth)
		if err != nil {
			continue
		}
		rows = append(rows, model.MacroMoneySupplyMonth{
			StatYear:        year,
			StatMonth:       month,
			M0Month:         parseFloatPtr(item.M0Month),
			M0Yoy:           parseFloatPtr(item.M0YOY),
			M0ChainRelative: parseFloatPtr(item.M0ChainRelative),
			M1Month:         parseFloatPtr(item.M1Month),
			M1Yoy:           parseFloatPtr(item.M1YOY),
			M1ChainRelative: parseFloatPtr(item.M1ChainRelative),
			M2Month:         parseFloatPtr(item.M2Month),
			M2Yoy:           parseFloatPtr(item.M2YOY),
			M2ChainRelative: parseFloatPtr(item.M2ChainRelative),
		})
	}
	if len(rows) == 0 {
		log.Printf("[StockSync] 货币供应量(月度)无数据")
		return nil
	}

	const batchSize = 500
	for i := 0; i < len(rows); i += batchSize {
		end := min(i+batchSize, len(rows))
		batch := rows[i:end]
		if err := DB.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "stat_year"}, {Name: "stat_month"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"m0_month", "m0_yoy", "m0_chain_relative",
				"m1_month", "m1_yoy", "m1_chain_relative",
				"m2_month", "m2_yoy", "m2_chain_relative",
			}),
		}).Create(&batch).Error; err != nil {
			return fmt.Errorf("写入货币供应量(月度)失败: %v", err)
		}
	}
	log.Printf("[StockSync] 货币供应量(月度)同步: %d 条", len(rows))
	return nil
}

// syncMoneySupplyYear 同步货币供应量年度数据(年底余额) (query_money_supply_data_year, 1次调用全量)
func (s *StockSyncService) syncMoneySupplyYear() error {
	url := fmt.Sprintf("%s/query_money_supply_data_year", s.baostockURL)
	body, err := s.httpGetWithDelay(url)
	if err != nil {
		return fmt.Errorf("获取货币供应量(年度)失败: %v", err)
	}

	var resp struct {
		Ok   bool `json:"ok"`
		Data struct {
			Items []struct {
				StatYear  string `json:"statYear"`
				M0Year    string `json:"m0Year"`
				M0YearYOY string `json:"m0YearYOY"`
				M1Year    string `json:"m1Year"`
				M1YearYOY string `json:"m1YearYOY"`
				M2Year    string `json:"m2Year"`
				M2YearYOY string `json:"m2YearYOY"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("解析货币供应量(年度)失败: %v", err)
	}
	if !resp.Ok {
		return fmt.Errorf("获取货币供应量(年度)失败")
	}

	rows := make([]model.MacroMoneySupplyYear, 0, len(resp.Data.Items))
	for _, item := range resp.Data.Items {
		year, err := strconv.Atoi(item.StatYear)
		if err != nil {
			continue
		}
		rows = append(rows, model.MacroMoneySupplyYear{
			StatYear:  year,
			M0Year:    parseFloatPtr(item.M0Year),
			M0YearYoy: parseFloatPtr(item.M0YearYOY),
			M1Year:    parseFloatPtr(item.M1Year),
			M1YearYoy: parseFloatPtr(item.M1YearYOY),
			M2Year:    parseFloatPtr(item.M2Year),
			M2YearYoy: parseFloatPtr(item.M2YearYOY),
		})
	}
	if len(rows) == 0 {
		log.Printf("[StockSync] 货币供应量(年度)无数据")
		return nil
	}

	if err := DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "stat_year"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"m0_year", "m0_year_yoy", "m1_year", "m1_year_yoy", "m2_year", "m2_year_yoy",
		}),
	}).Create(&rows).Error; err != nil {
		return fmt.Errorf("写入货币供应量(年度)失败: %v", err)
	}
	log.Printf("[StockSync] 货币供应量(年度)同步: %d 条", len(rows))
	return nil
}

// syncLPR 同步贷款市场报价利率 LPR (query_lpr, 1次调用全量, 底层东财 RPTA_WEB_RATE)
// 包含 LPR1Y/LPR5Y(2019-08 起) 与基准贷款利率 RATE_1/RATE_2(1991 起)
func (s *StockSyncService) syncLPR() error {
	url := fmt.Sprintf("%s/query_lpr", s.baostockURL)
	body, err := s.httpGetWithDelay(url)
	if err != nil {
		return fmt.Errorf("获取LPR失败: %v", err)
	}

	var resp struct {
		Ok   bool `json:"ok"`
		Data struct {
			Items []struct {
				TRADE_DATE string `json:"TRADE_DATE"`
				LPR1Y      string `json:"LPR1Y"`
				LPR5Y      string `json:"LPR5Y"`
				RATE_1     string `json:"RATE_1"`
				RATE_2     string `json:"RATE_2"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("解析LPR失败: %v", err)
	}
	if !resp.Ok {
		return fmt.Errorf("获取LPR失败")
	}

	rows := make([]model.MacroLPR, 0, len(resp.Data.Items))
	for _, item := range resp.Data.Items {
		tradeDate, err := parseTradeDay(item.TRADE_DATE)
		if err != nil {
			continue
		}
		rows = append(rows, model.MacroLPR{
			TradeDate: tradeDate,
			Lpr1Y:     parseFloatPtr(item.LPR1Y),
			Lpr5Y:     parseFloatPtr(item.LPR5Y),
			Rate1:     parseFloatPtr(item.RATE_1),
			Rate2:     parseFloatPtr(item.RATE_2),
		})
	}
	if len(rows) == 0 {
		log.Printf("[StockSync] LPR无数据")
		return nil
	}

	const batchSize = 500
	for i := 0; i < len(rows); i += batchSize {
		end := min(i+batchSize, len(rows))
		batch := rows[i:end]
		if err := DB.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "trade_date"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"lpr_1y", "lpr_5y", "rate_1", "rate_2",
			}),
		}).Create(&batch).Error; err != nil {
			return fmt.Errorf("写入LPR失败: %v", err)
		}
	}
	log.Printf("[StockSync] LPR同步: %d 条", len(rows))
	return nil
}

// syncGDP 同步GDP数据 (query_gdp, 1次调用全量)
func (s *StockSyncService) syncGDP() error {
	url := fmt.Sprintf("%s/query_gdp", s.baostockURL)
	body, err := s.httpGetWithDelay(url)
	if err != nil {
		return fmt.Errorf("获取GDP失败: %v", err)
	}

	var resp struct {
		Ok   bool `json:"ok"`
		Data struct {
			Items []struct {
				Quarter           string `json:"季度"`
				GdpAbsolute       string `json:"国内生产总值-绝对值"`
				GdpYoy            string `json:"国内生产总值-同比增长"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("解析GDP失败: %v", err)
	}
	if !resp.Ok {
		return fmt.Errorf("获取GDP失败")
	}

	rows := make([]model.MacroGDP, 0, len(resp.Data.Items))
	for _, item := range resp.Data.Items {
		if item.Quarter == "" {
			continue
		}
		rows = append(rows, model.MacroGDP{
			Quarter:    item.Quarter,
			GdpYoy:     parseFloatPtr(item.GdpYoy),
			GdpCumulative: parseFloatPtr(item.GdpAbsolute),
		})
	}
	if len(rows) == 0 {
		log.Printf("[StockSync] GDP无数据")
		return nil
	}

	if err := DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "quarter"}},
		DoUpdates: clause.AssignmentColumns([]string{"gdp_yoy", "gdp_cumulative"}),
	}).Create(&rows).Error; err != nil {
		return fmt.Errorf("写入GDP失败: %v", err)
	}
	log.Printf("[StockSync] GDP同步: %d 条", len(rows))
	return nil
}

// syncCPI 同步CPI数据 (query_cpi, 1次调用全量)
func (s *StockSyncService) syncCPI() error {
	url := fmt.Sprintf("%s/query_cpi", s.baostockURL)
	body, err := s.httpGetWithDelay(url)
	if err != nil {
		return fmt.Errorf("获取CPI失败: %v", err)
	}

	var resp struct {
		Ok   bool `json:"ok"`
		Data struct {
			Items []struct {
				Month          string `json:"月份"`
				CpiCurrent     string `json:"全国-当月"`
				CpiYoy         string `json:"全国-同比增长"`
				CpiMom         string `json:"全国-环比增长"`
				CpiCumulative  string `json:"全国-累计"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("解析CPI失败: %v", err)
	}
	if !resp.Ok {
		return fmt.Errorf("获取CPI失败")
	}

	rows := make([]model.MacroCPI, 0, len(resp.Data.Items))
	for _, item := range resp.Data.Items {
		if item.Month == "" {
			continue
		}
		rows = append(rows, model.MacroCPI{
			Month:            item.Month,
			CpiYoy:           parseFloatPtr(item.CpiYoy),
			CpiMom:           parseFloatPtr(item.CpiMom),
			CpiCumulativeYoy: parseFloatPtr(item.CpiCumulative),
		})
	}
	if len(rows) == 0 {
		log.Printf("[StockSync] CPI无数据")
		return nil
	}

	if err := DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "month"}},
		DoUpdates: clause.AssignmentColumns([]string{"cpi_yoy", "cpi_mom", "cpi_cumulative_yoy"}),
	}).Create(&rows).Error; err != nil {
		return fmt.Errorf("写入CPI失败: %v", err)
	}
	log.Printf("[StockSync] CPI同步: %d 条", len(rows))
	return nil
}

// syncPMI 同步PMI数据 (query_pmi, 1次调用全量)
func (s *StockSyncService) syncPMI() error {
	url := fmt.Sprintf("%s/query_pmi", s.baostockURL)
	body, err := s.httpGetWithDelay(url)
	if err != nil {
		return fmt.Errorf("获取PMI失败: %v", err)
	}

	var resp struct {
		Ok   bool `json:"ok"`
		Data struct {
			Items []struct {
				Month                  string `json:"月份"`
				PmiManufacturing       string `json:"制造业-指数"`
				PmiManufacturingYoy    string `json:"制造业-同比增长"`
				PmiNonManufacturing    string `json:"非制造业-指数"`
				PmiNonManufacturingYoy string `json:"非制造业-同比增长"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("解析PMI失败: %v", err)
	}
	if !resp.Ok {
		return fmt.Errorf("获取PMI失败")
	}

	rows := make([]model.MacroPMI, 0, len(resp.Data.Items))
	for _, item := range resp.Data.Items {
		if item.Month == "" {
			continue
		}
		rows = append(rows, model.MacroPMI{
			Month:                  item.Month,
			PmiManufacturing:       parseFloatPtr(item.PmiManufacturing),
			PmiManufacturingYoy:    parseFloatPtr(item.PmiManufacturingYoy),
			PmiNonManufacturing:    parseFloatPtr(item.PmiNonManufacturing),
			PmiNonManufacturingYoy: parseFloatPtr(item.PmiNonManufacturingYoy),
		})
	}
	if len(rows) == 0 {
		log.Printf("[StockSync] PMI无数据")
		return nil
	}

	if err := DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "month"}},
		DoUpdates: clause.AssignmentColumns([]string{"pmi_manufacturing", "pmi_manufacturing_yoy", "pmi_non_manufacturing", "pmi_non_manufacturing_yoy"}),
	}).Create(&rows).Error; err != nil {
		return fmt.Errorf("写入PMI失败: %v", err)
	}
	log.Printf("[StockSync] PMI同步: %d 条", len(rows))
	return nil
}

// syncPPI 同步PPI数据 (query_ppi, 1次调用全量)
func (s *StockSyncService) syncPPI() error {
	url := fmt.Sprintf("%s/query_ppi", s.baostockURL)
	body, err := s.httpGetWithDelay(url)
	if err != nil {
		return fmt.Errorf("获取PPI失败: %v", err)
	}

	var resp struct {
		Ok   bool `json:"ok"`
		Data struct {
			Items []struct {
				Month          string `json:"月份"`
				PpiCurrent     string `json:"当月"`
				PpiYoy         string `json:"当月同比增长"`
				PpiCumulative  string `json:"累计"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("解析PPI失败: %v", err)
	}
	if !resp.Ok {
		return fmt.Errorf("获取PPI失败")
	}

	rows := make([]model.MacroPPI, 0, len(resp.Data.Items))
	for _, item := range resp.Data.Items {
		if item.Month == "" {
			continue
		}
		rows = append(rows, model.MacroPPI{
			Month:            item.Month,
			PpiYoy:           parseFloatPtr(item.PpiYoy),
			PpiCumulativeYoy: parseFloatPtr(item.PpiCumulative),
		})
	}
	if len(rows) == 0 {
		log.Printf("[StockSync] PPI无数据")
		return nil
	}

	if err := DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "month"}},
		DoUpdates: clause.AssignmentColumns([]string{"ppi_yoy", "ppi_cumulative_yoy"}),
	}).Create(&rows).Error; err != nil {
		return fmt.Errorf("写入PPI失败: %v", err)
	}
	log.Printf("[StockSync] PPI同步: %d 条", len(rows))
	return nil
}
