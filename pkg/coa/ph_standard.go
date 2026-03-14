package coa

// AccountTemplate defines a COA account for seeding.
type AccountTemplate struct {
	Number      string
	Name        string
	Type        string // asset, liability, equity, revenue, expense
	SubType     string
	Normal      string // debit, credit
	Description string
}

// PHStandardCOA returns the Philippine BIR standard chart of accounts (~50 accounts).
func PHStandardCOA() []AccountTemplate {
	return []AccountTemplate{
		// ============================================================
		// 1xxx — ASSETS
		// ============================================================
		{"1000", "Cash on Hand", "asset", "cash", "debit", "Petty cash and cash on hand"},
		{"1010", "Cash in Bank", "asset", "cash", "debit", "Checking and savings accounts"},
		{"1020", "Cash Equivalents", "asset", "cash", "debit", "Money market, short-term investments"},
		{"1100", "Accounts Receivable - Trade", "asset", "receivable", "debit", "Trade receivables from customers"},
		{"1110", "Allowance for Doubtful Accounts", "asset", "receivable", "credit", "Contra-asset for bad debts"},
		{"1200", "Inventory - Merchandise", "asset", "inventory", "debit", "Goods held for sale"},
		{"1210", "Inventory - Raw Materials", "asset", "inventory", "debit", "Raw materials for production"},
		{"1300", "Prepaid Expenses", "asset", "prepaid", "debit", "Prepaid rent, insurance, etc."},
		{"1310", "Advances to Employees", "asset", "prepaid", "debit", "Employee advances and loans"},
		{"1400", "Input VAT", "asset", "tax_credit", "debit", "VAT on purchases (creditable)"},
		{"1410", "Creditable Withholding Tax", "asset", "tax_credit", "debit", "CWT from customers (BIR 2307)"},
		{"1420", "Tax Credits Applied", "asset", "tax_credit", "debit", "Excess tax credits carried forward"},
		{"1500", "Property, Plant & Equipment", "asset", "fixed", "debit", "Land, buildings, machinery"},
		{"1510", "Accumulated Depreciation", "asset", "fixed", "credit", "Contra-asset for depreciation"},
		{"1600", "Intangible Assets", "asset", "intangible", "debit", "Software, licenses, goodwill"},
		{"1610", "Accumulated Amortization", "asset", "intangible", "credit", "Contra-asset for amortization"},

		// ============================================================
		// 2xxx — LIABILITIES
		// ============================================================
		{"2000", "Accounts Payable - Trade", "liability", "payable", "credit", "Trade payables to suppliers"},
		{"2010", "Accrued Expenses", "liability", "payable", "credit", "Accrued salaries, utilities, etc."},
		{"2100", "Notes Payable", "liability", "notes", "credit", "Short-term borrowings"},
		{"2200", "Output VAT", "liability", "tax_payable", "credit", "VAT on sales (payable to BIR)"},
		{"2210", "Withholding Tax Payable - Expanded", "liability", "tax_payable", "credit", "EWT on payments to suppliers"},
		{"2220", "Withholding Tax Payable - Compensation", "liability", "tax_payable", "credit", "Tax withheld from employee salaries"},
		{"2230", "Income Tax Payable", "liability", "tax_payable", "credit", "Corporate income tax payable"},
		{"2240", "Percentage Tax Payable", "liability", "tax_payable", "credit", "Percentage tax for non-VAT entities"},
		{"2300", "SSS/PhilHealth/HDMF Payable", "liability", "statutory", "credit", "Statutory contributions payable"},
		{"2400", "Long-term Debt", "liability", "long_term", "credit", "Bank loans, mortgages"},
		{"2500", "Unearned Revenue", "liability", "deferred", "credit", "Advance payments from customers"},

		// ============================================================
		// 3xxx — EQUITY
		// ============================================================
		{"3000", "Common Stock / Capital", "equity", "capital", "credit", "Owner's capital / share capital"},
		{"3100", "Retained Earnings", "equity", "retained", "credit", "Accumulated profits/losses"},
		{"3200", "Drawings / Dividends", "equity", "drawings", "debit", "Owner's withdrawals / dividends declared"},
		{"3300", "Opening Balance Equity", "equity", "opening", "credit", "Opening balance adjustments"},
		{"3900", "Income Summary", "equity", "closing", "credit", "Year-end closing temporary account"},

		// ============================================================
		// 4xxx — REVENUE (by VAT classification)
		// ============================================================
		{"4000", "Sales Revenue - Vatable", "revenue", "vatable", "credit", "Sales subject to 12% VAT"},
		{"4010", "Sales Revenue - Zero-Rated", "revenue", "zero_rated", "credit", "Sales subject to 0% VAT (exports)"},
		{"4020", "Sales Revenue - VAT Exempt", "revenue", "exempt", "credit", "Sales exempt from VAT"},
		{"4100", "Service Revenue - Vatable", "revenue", "vatable", "credit", "Service income subject to 12% VAT"},
		{"4110", "Service Revenue - Zero-Rated", "revenue", "zero_rated", "credit", "Service income at 0% VAT"},
		{"4200", "Other Income", "revenue", "other", "credit", "Interest, gains, miscellaneous income"},
		{"4210", "Sales Returns & Allowances", "revenue", "contra", "debit", "Contra-revenue for returns"},
		{"4220", "Sales Discounts", "revenue", "contra", "debit", "Contra-revenue for discounts given"},

		// ============================================================
		// 5xxx — COST OF GOODS SOLD
		// ============================================================
		{"5000", "Cost of Goods Sold", "expense", "cogs", "debit", "Direct cost of goods sold"},
		{"5010", "Cost of Services", "expense", "cogs", "debit", "Direct cost of services rendered"},
		{"5020", "Freight & Delivery", "expense", "cogs", "debit", "Shipping and delivery costs"},

		// ============================================================
		// 6xxx — OPERATING EXPENSES
		// ============================================================
		{"6000", "Salaries & Wages", "expense", "payroll", "debit", "Employee salaries and wages"},
		{"6010", "Employee Benefits", "expense", "payroll", "debit", "SSS, PhilHealth, HDMF, 13th month"},
		{"6100", "Rent Expense", "expense", "occupancy", "debit", "Office/warehouse rent"},
		{"6110", "Utilities Expense", "expense", "occupancy", "debit", "Electricity, water, internet"},
		{"6200", "Office Supplies", "expense", "admin", "debit", "Stationery, printing supplies"},
		{"6210", "Repairs & Maintenance", "expense", "admin", "debit", "Equipment and facility repairs"},
		{"6300", "Professional Fees", "expense", "professional", "debit", "Legal, accounting, consulting fees"},
		{"6310", "Advertising & Marketing", "expense", "marketing", "debit", "Ads, promotions, marketing"},
		{"6400", "Transportation & Travel", "expense", "travel", "debit", "Business travel and transportation"},
		{"6500", "Depreciation Expense", "expense", "depreciation", "debit", "Depreciation of fixed assets"},
		{"6510", "Amortization Expense", "expense", "amortization", "debit", "Amortization of intangibles"},
		{"6600", "Insurance Expense", "expense", "insurance", "debit", "Business insurance premiums"},
		{"6700", "Taxes & Licenses", "expense", "taxes", "debit", "Business permits, local taxes"},
		{"6800", "Bad Debts Expense", "expense", "bad_debts", "debit", "Write-off of uncollectible accounts"},
		{"6900", "Miscellaneous Expense", "expense", "misc", "debit", "Other operating expenses"},
	}
}
