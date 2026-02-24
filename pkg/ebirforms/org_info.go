package ebirforms

// OrgInfo holds the taxpayer information needed for eBIRForms DAT export.
type OrgInfo struct {
	TIN            string `json:"tin"`
	RegisteredName string `json:"registered_name"`
	TradeName      string `json:"trade_name"`
	RDOCode        string `json:"rdo_code"`
	ZipCode        string `json:"zip_code"`
	Address        string `json:"address"`
	TaxPayerType   string `json:"taxpayer_type"` // "I" = Individual, "C" = Corporation
	ContactNumber  string `json:"contact_number"`
	Email          string `json:"email"`
}
