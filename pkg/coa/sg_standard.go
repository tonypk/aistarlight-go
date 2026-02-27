package coa

// SGStandardCOA returns the Singapore SFRS(I) / ACRA standard chart of accounts.
func SGStandardCOA() []AccountTemplate {
	return []AccountTemplate{
		// ============================================================
		// 1xxx — ASSETS
		// ============================================================
		{"1000", "Cash on Hand", "asset", "cash", "debit", "Petty cash and cash on hand"},
		{"1010", "Cash in Bank", "asset", "cash", "debit", "Current and savings accounts"},
		{"1020", "Cash Equivalents", "asset", "cash", "debit", "Fixed deposits, money market funds"},
		{"1100", "Accounts Receivable - Trade", "asset", "receivable", "debit", "Trade receivables from customers"},
		{"1110", "Allowance for Doubtful Accounts", "asset", "receivable", "credit", "Contra-asset for expected credit losses (SFRS(I) 9)"},
		{"1200", "Inventory - Merchandise", "asset", "inventory", "debit", "Goods held for sale"},
		{"1210", "Inventory - Raw Materials", "asset", "inventory", "debit", "Raw materials for production"},
		{"1300", "Prepaid Expenses", "asset", "prepaid", "debit", "Prepaid rent, insurance, etc."},
		{"1310", "Deposits & Advances", "asset", "prepaid", "debit", "Rental deposits, utility deposits, staff advances"},
		{"1400", "GST Input Tax", "asset", "tax_credit", "debit", "GST paid on purchases (claimable)"},
		{"1410", "Withholding Tax Recoverable", "asset", "tax_credit", "debit", "S45 WHT overpayments recoverable"},
		{"1420", "Tax Credits Applied", "asset", "tax_credit", "debit", "Excess tax credits carried forward"},
		{"1500", "Property, Plant & Equipment", "asset", "fixed", "debit", "Land, buildings, machinery, office equipment"},
		{"1510", "Accumulated Depreciation", "asset", "fixed", "credit", "Contra-asset for depreciation"},
		{"1600", "Intangible Assets", "asset", "intangible", "debit", "Software, licences, goodwill"},
		{"1610", "Accumulated Amortisation", "asset", "intangible", "credit", "Contra-asset for amortisation"},
		{"1700", "Right-of-Use Assets", "asset", "rou", "debit", "Leased assets under SFRS(I) 16"},
		{"1710", "Accumulated Depreciation - ROU", "asset", "rou", "credit", "Depreciation of right-of-use assets"},

		// ============================================================
		// 2xxx — LIABILITIES
		// ============================================================
		{"2000", "Accounts Payable - Trade", "liability", "payable", "credit", "Trade payables to suppliers"},
		{"2010", "Accrued Expenses", "liability", "payable", "credit", "Accrued salaries, utilities, etc."},
		{"2100", "Bank Loans & Overdrafts", "liability", "notes", "credit", "Short-term borrowings and overdraft facilities"},
		{"2200", "GST Output Tax", "liability", "tax_payable", "credit", "GST collected on sales (payable to IRAS)"},
		{"2210", "Withholding Tax Payable", "liability", "tax_payable", "credit", "S45 withholding tax on non-resident payments"},
		{"2220", "Corporate Income Tax Payable", "liability", "tax_payable", "credit", "Corporate tax payable at 17%"},
		{"2230", "Provision for Tax", "liability", "tax_payable", "credit", "Estimated tax provision (current year)"},
		{"2300", "CPF Payable", "liability", "statutory", "credit", "Central Provident Fund contributions payable"},
		{"2310", "SDL Payable", "liability", "statutory", "credit", "Skills Development Levy (0.25% of gross wages, min S$2)"},
		{"2320", "FWL Payable", "liability", "statutory", "credit", "Foreign Worker Levy payable"},
		{"2400", "Long-term Borrowings", "liability", "long_term", "credit", "Bank term loans, bonds"},
		{"2500", "Deferred Revenue", "liability", "deferred", "credit", "Advance payments from customers (SFRS(I) 15)"},
		{"2600", "Lease Liabilities", "liability", "lease", "credit", "Lease obligations under SFRS(I) 16"},

		// ============================================================
		// 3xxx — EQUITY
		// ============================================================
		{"3000", "Share Capital", "equity", "capital", "credit", "Issued and paid-up share capital"},
		{"3100", "Retained Earnings", "equity", "retained", "credit", "Accumulated profits / losses"},
		{"3200", "Dividends Declared", "equity", "drawings", "debit", "Dividends declared to shareholders"},

		// ============================================================
		// 4xxx — REVENUE (by GST classification)
		// ============================================================
		{"4000", "Sales Revenue - Standard-Rated", "revenue", "standard_rated", "credit", "Sales subject to 9% GST"},
		{"4010", "Sales Revenue - Zero-Rated", "revenue", "zero_rated", "credit", "Export sales at 0% GST"},
		{"4020", "Sales Revenue - Exempt", "revenue", "exempt", "credit", "Exempt supplies (financial services, residential property)"},
		{"4030", "Sales Revenue - Out-of-Scope", "revenue", "out_of_scope", "credit", "Out-of-scope supplies (private transactions, overseas)"},
		{"4100", "Service Revenue - Standard-Rated", "revenue", "standard_rated", "credit", "Service income subject to 9% GST"},
		{"4110", "Service Revenue - Zero-Rated", "revenue", "zero_rated", "credit", "International services at 0% GST"},
		{"4200", "Other Operating Income", "revenue", "other", "credit", "Interest, gains, government grants"},
		{"4210", "Sales Returns & Allowances", "revenue", "contra", "debit", "Contra-revenue for returns"},
		{"4220", "Sales Discounts", "revenue", "contra", "debit", "Contra-revenue for discounts given"},

		// ============================================================
		// 5xxx — COST OF SALES
		// ============================================================
		{"5000", "Cost of Goods Sold", "expense", "cogs", "debit", "Direct cost of goods sold"},
		{"5010", "Cost of Services", "expense", "cogs", "debit", "Direct cost of services rendered"},
		{"5020", "Freight & Delivery", "expense", "cogs", "debit", "Shipping and delivery costs"},
		{"5030", "Import Duties & Customs", "expense", "cogs", "debit", "Import duties and customs clearance fees"},

		// ============================================================
		// 6xxx — OPERATING EXPENSES
		// ============================================================
		{"6000", "Salaries & Wages", "expense", "payroll", "debit", "Employee salaries and wages"},
		{"6010", "CPF Contributions (Employer)", "expense", "payroll", "debit", "Employer CPF contributions (up to 17% for ≤55 yrs)"},
		{"6020", "SDL & FWL Contributions", "expense", "payroll", "debit", "Skills Development Levy and Foreign Worker Levy"},
		{"6030", "Staff Welfare & Benefits", "expense", "payroll", "debit", "Medical, dental, training, staff amenities"},
		{"6100", "Rent Expense", "expense", "occupancy", "debit", "Office and warehouse rent"},
		{"6110", "Utilities Expense", "expense", "occupancy", "debit", "Electricity, water, internet, telephone"},
		{"6200", "Office Supplies & Stationery", "expense", "admin", "debit", "Stationery, printing supplies"},
		{"6210", "Repairs & Maintenance", "expense", "admin", "debit", "Equipment and facility repairs"},
		{"6300", "Professional Fees", "expense", "professional", "debit", "Legal, audit, consulting, company secretary fees"},
		{"6310", "Advertising & Marketing", "expense", "marketing", "debit", "Ads, promotions, digital marketing"},
		{"6400", "Transportation & Travel", "expense", "travel", "debit", "Business travel and transportation"},
		{"6500", "Depreciation Expense", "expense", "depreciation", "debit", "Depreciation of fixed assets and ROU assets"},
		{"6510", "Amortisation Expense", "expense", "amortization", "debit", "Amortisation of intangible assets"},
		{"6600", "Insurance Expense", "expense", "insurance", "debit", "Business insurance premiums"},
		{"6700", "Taxes & Licences", "expense", "taxes", "debit", "Business licences, property tax (excl. income tax)"},
		{"6800", "Bad Debts Expense", "expense", "bad_debts", "debit", "Write-off of uncollectible accounts"},
		{"6900", "Miscellaneous Expense", "expense", "misc", "debit", "Other operating expenses"},
		{"6910", "Bank Charges", "expense", "finance", "debit", "Bank service fees, transaction charges"},
		{"6920", "Foreign Exchange Loss", "expense", "finance", "debit", "Realised and unrealised FX losses"},
	}
}
