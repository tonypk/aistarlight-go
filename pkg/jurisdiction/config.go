package jurisdiction

// Config holds country-specific configuration for the receipt pipeline.
type Config struct {
	Code            string   // "PH", "SG", "LK"
	Name            string   // "Philippines", "Singapore", "Sri Lanka"
	Currency        string   // "PHP", "SGD", "LKR"
	CurrencySymbol  string   // "₱", "S$", "Rs"
	TINPattern      string   // regex for tax ID
	TINName         string   // "TIN", "UEN", "TIN/VAT ID"
	VATRate         float64  // 0.12, 0.09, 0.18
	VATName         string   // "VAT", "GST", "VAT"
	VATTypes        []string // valid classification types
	Categories      []string // valid categories
	AmountPatterns  []string // currency regex patterns for receipt parsing
	TotalLabels     []string // receipt labels for total amount
	VATLabels       []string // receipt labels for VAT amount
	VatableLabels   []string // receipt labels for vatable sales
	ExemptLabels    []string // receipt labels for exempt items
	ZeroRatedLabels []string // receipt labels for zero-rated items
	GovtKeywords    []string // government entity keywords
	GovtTINPrefixes []string // TIN prefixes indicating government
	ClassPrompt     string   // LLM system prompt for classification
	DefaultReport   string   // default form/report type
}

var configs = map[string]Config{
	"PH": phConfig(),
	"SG": sgConfig(),
	"LK": lkConfig(),
}

// Get returns the jurisdiction config for the given code, defaulting to PH.
func Get(code string) Config {
	if cfg, ok := configs[code]; ok {
		return cfg
	}
	return configs["PH"]
}

// All returns all supported jurisdiction configs.
func All() []Config {
	return []Config{configs["PH"], configs["SG"], configs["LK"]}
}

func phConfig() Config {
	return Config{
		Code:           "PH",
		Name:           "Philippines",
		Currency:       "PHP",
		CurrencySymbol: "₱",
		TINPattern:     `\d{3}[-\s]?\d{3}[-\s]?\d{3}[-\s]?\d{3,4}`,
		TINName:        "TIN",
		VATRate:        0.12,
		VATName:        "VAT",
		VATTypes:       []string{"vatable", "exempt", "zero_rated", "government"},
		Categories:     []string{"goods", "services", "capital", "imports", "sale"},
		AmountPatterns: []string{`(?i)(?:₱|PHP|PhP|Php|P)\s*([\d,]+\.\d{2})`},
		TotalLabels:    []string{"TOTAL", "TOTAL AMOUNT", "GRAND TOTAL", "AMOUNT DUE", "NET AMOUNT"},
		VATLabels:      []string{"VAT AMOUNT", "VAT", "OUTPUT TAX", "12% VAT"},
		VatableLabels:  []string{"VATABLE SALES", "VATABLE", "VAT SALES"},
		ExemptLabels:   []string{"VAT EXEMPT", "EXEMPT SALES", "VAT-EXEMPT"},
		ZeroRatedLabels: []string{"ZERO RATED", "ZERO-RATED", "0% VAT"},
		GovtKeywords:   []string{"bir", "sss", "philhealth", "pag-ibig", "hdmf", "lgu", "municipality", "dti", "sec"},
		GovtTINPrefixes: []string{"000", "001", "002"},
		ClassPrompt: `Expert Philippine tax accountant. Classify each transaction for BIR VAT filing.

For each transaction, determine:
1. vat_type: "vatable" | "exempt" | "zero_rated" | "government"
2. category: "goods" | "services" | "capital" | "imports" | "sale"
3. confidence: 0.00-1.00

IMPORTANT: Each transaction has a "source_type" field:
- "sales_record": This is a SALES transaction. Use category="sale" for domestic/export/government sales.
- "purchase_record": This is a PURCHASE transaction. Use category="goods", "services", "capital", or "imports". NEVER use category="sale" for purchases.

Rules:
- Sales to government: vat_type="government", category="sale" (only if source_type="sales_record")
- Export sales: vat_type="zero_rated", category="sale" (only if source_type="sales_record")
- Domestic sales: vat_type="vatable", category="sale" (only if source_type="sales_record")
- Purchases from government: vat_type="government", category="goods" or "services"
- Equipment/machinery > 1M PHP: category="capital"
- Imported goods: category="imports"
- Agricultural/exempt: vat_type="exempt"

Respond ONLY with valid JSON array: [{"index": 0, "vat_type": "...", "category": "...", "confidence": 0.90}, ...]`,
		DefaultReport: "BIR_2550M",
	}
}

func sgConfig() Config {
	return Config{
		Code:           "SG",
		Name:           "Singapore",
		Currency:       "SGD",
		CurrencySymbol: "S$",
		TINPattern:     `(?:\d{9}[A-Z]|\d{8}[A-Z]|[A-Z]\d{7}[A-Z])`,
		TINName:        "UEN",
		VATRate:        0.09,
		VATName:        "GST",
		VATTypes:       []string{"standard", "zero_rated", "exempt", "out_of_scope"},
		Categories:     []string{"goods", "services", "capital", "imports", "sale"},
		AmountPatterns: []string{`(?i)(?:S\$|SGD)\s*([\d,]+\.\d{2})`},
		TotalLabels:    []string{"TOTAL", "TOTAL AMOUNT", "GRAND TOTAL", "AMOUNT DUE", "NET AMOUNT", "TOTAL (SGD)"},
		VATLabels:      []string{"GST AMOUNT", "GST", "9% GST", "GST 9%", "TAX"},
		VatableLabels:  []string{"TAXABLE AMOUNT", "AMOUNT BEFORE GST", "SUBTOTAL"},
		ExemptLabels:   []string{"GST EXEMPT", "EXEMPT", "OUT OF SCOPE"},
		ZeroRatedLabels: []string{"ZERO RATED", "ZERO-RATED", "0% GST", "EXPORT"},
		GovtKeywords:   []string{"iras", "cpf", "mof", "government", "ministry", "singapore customs"},
		GovtTINPrefixes: []string{"T08"},
		ClassPrompt: `Expert Singapore tax accountant. Classify each transaction for IRAS GST filing.

For each transaction, determine:
1. vat_type: "standard" | "zero_rated" | "exempt" | "out_of_scope"
2. category: "goods" | "services" | "capital" | "imports" | "sale"
3. confidence: 0.00-1.00

IMPORTANT: Each transaction has a "source_type" field:
- "sales_record": This is a SALES transaction. Use category="sale".
- "purchase_record": This is a PURCHASE transaction. Use category="goods", "services", "capital", or "imports". NEVER use category="sale" for purchases.

Rules:
- Standard-rated supplies: vat_type="standard" (9% GST)
- Zero-rated exports: vat_type="zero_rated"
- Exempt financial services: vat_type="exempt"
- Imported goods: category="imports"
- Capital equipment > 10K SGD: category="capital"

Respond ONLY with valid JSON array: [{"index": 0, "vat_type": "...", "category": "...", "confidence": 0.90}, ...]`,
		DefaultReport: "IRAS_GST_F5",
	}
}

func lkConfig() Config {
	return Config{
		Code:           "LK",
		Name:           "Sri Lanka",
		Currency:       "LKR",
		CurrencySymbol: "Rs",
		TINPattern:     `[A-Z]?\d{9,13}`,
		TINName:        "TIN/VAT ID",
		VATRate:        0.18,
		VATName:        "VAT",
		VATTypes:       []string{"standard", "reduced", "exempt", "svat"},
		Categories:     []string{"goods", "services", "capital", "imports", "sale"},
		AmountPatterns: []string{`(?i)(?:Rs\.?|LKR)\s*([\d,]+\.\d{2})`},
		TotalLabels:    []string{"TOTAL", "TOTAL AMOUNT", "GRAND TOTAL", "AMOUNT DUE", "NET AMOUNT", "TOTAL (LKR)"},
		VATLabels:      []string{"VAT AMOUNT", "VAT", "18% VAT", "VAT 18%", "OUTPUT VAT"},
		VatableLabels:  []string{"TAXABLE AMOUNT", "AMOUNT BEFORE VAT", "SUBTOTAL", "NET"},
		ExemptLabels:   []string{"VAT EXEMPT", "EXEMPT"},
		ZeroRatedLabels: []string{"ZERO RATED", "ZERO-RATED", "0% VAT", "SVAT", "SUSPENDED VAT"},
		GovtKeywords:   []string{"inland revenue", "customs", "government", "ministry", "sri lanka", "dept of"},
		GovtTINPrefixes: []string{},
		ClassPrompt: `Expert Sri Lanka tax accountant. Classify each transaction for Inland Revenue VAT filing.

For each transaction, determine:
1. vat_type: "standard" | "reduced" | "exempt" | "svat"
2. category: "goods" | "services" | "capital" | "imports" | "sale"
3. confidence: 0.00-1.00

IMPORTANT: Each transaction has a "source_type" field:
- "sales_record": This is a SALES transaction. Use category="sale".
- "purchase_record": This is a PURCHASE transaction. Use category="goods", "services", "capital", or "imports". NEVER use category="sale" for purchases.

Rules:
- Standard-rated supplies: vat_type="standard" (18% VAT)
- SVAT scheme (suspended VAT for exporters): vat_type="svat"
- Exempt items (unprocessed food, medical, education): vat_type="exempt"
- Imported goods: category="imports"
- Capital equipment: category="capital"
- Government purchases: check TIN or keywords

Respond ONLY with valid JSON array: [{"index": 0, "vat_type": "...", "category": "...", "confidence": 0.90}, ...]`,
		DefaultReport: "LK_VAT_RETURN",
	}
}
