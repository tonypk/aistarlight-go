package qbo

import "time"

// TokenResponse represents the OAuth 2.0 token response from Intuit.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	// Intuit refresh tokens expire after 180 days (not in response, computed).
	RefreshExpiresIn int `json:"x_refresh_token_expires_in"`
}

// QBOAccount represents a Chart of Accounts item from QBO.
type QBOAccount struct {
	ID                string  `json:"Id"`
	Name              string  `json:"Name"`
	AccountType       string  `json:"AccountType"`
	AccountSubType    string  `json:"AccountSubType"`
	Classification    string  `json:"Classification"`
	CurrentBalance    float64 `json:"CurrentBalance"`
	CurrencyRef       *Ref    `json:"CurrencyRef,omitempty"`
	Active            bool    `json:"Active"`
	FullyQualifiedName string `json:"FullyQualifiedName"`
	AcctNum           string  `json:"AcctNum"`
}

// QBOInvoice represents an Invoice from QBO.
type QBOInvoice struct {
	ID          string      `json:"Id"`
	DocNumber   string      `json:"DocNumber"`
	TxnDate     string      `json:"TxnDate"`
	DueDate     string      `json:"DueDate"`
	TotalAmt    float64     `json:"TotalAmt"`
	Balance     float64     `json:"Balance"`
	CustomerRef *Ref        `json:"CustomerRef,omitempty"`
	Line        []QBOLine   `json:"Line"`
	MetaData    QBOMetaData `json:"MetaData"`
}

// QBOBill represents a Bill (AP) from QBO.
type QBOBill struct {
	ID          string      `json:"Id"`
	DocNumber   string      `json:"DocNumber"`
	TxnDate     string      `json:"TxnDate"`
	DueDate     string      `json:"DueDate"`
	TotalAmt    float64     `json:"TotalAmt"`
	Balance     float64     `json:"Balance"`
	VendorRef   *Ref        `json:"VendorRef,omitempty"`
	Line        []QBOLine   `json:"Line"`
	MetaData    QBOMetaData `json:"MetaData"`
}

// QBOLine represents a transaction line item.
type QBOLine struct {
	ID                    string                 `json:"Id"`
	LineNum               int                    `json:"LineNum"`
	Amount                float64                `json:"Amount"`
	Description           string                 `json:"Description"`
	DetailType            string                 `json:"DetailType"`
	SalesItemLineDetail   map[string]interface{} `json:"SalesItemLineDetail,omitempty"`
	AccountBasedExpDetail map[string]interface{} `json:"AccountBasedExpenseLineDetail,omitempty"`
}

// QBOMetaData holds create/update timestamps.
type QBOMetaData struct {
	CreateTime      time.Time `json:"CreateTime"`
	LastUpdatedTime time.Time `json:"LastUpdatedTime"`
}

// Ref is a QBO entity reference.
type Ref struct {
	Value string `json:"value"`
	Name  string `json:"name"`
}

// QueryResponse wraps a QBO query result.
type QueryResponse struct {
	QueryResponse struct {
		Account     []QBOAccount `json:"Account,omitempty"`
		Invoice     []QBOInvoice `json:"Invoice,omitempty"`
		Bill        []QBOBill    `json:"Bill,omitempty"`
		StartPos    int          `json:"startPosition"`
		MaxResults  int          `json:"maxResults"`
		TotalCount  int          `json:"totalCount"`
	} `json:"QueryResponse"`
}

// ErrorResponse represents a QBO API error.
type ErrorResponse struct {
	Fault struct {
		Error []struct {
			Message string `json:"Message"`
			Detail  string `json:"Detail"`
			Code    string `json:"code"`
		} `json:"Error"`
		Type string `json:"type"`
	} `json:"Fault"`
}
