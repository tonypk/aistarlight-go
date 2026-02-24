package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/tonypk/aistarlight-go/internal/domain"
	"github.com/tonypk/aistarlight-go/internal/platform/crypto"
	"github.com/tonypk/aistarlight-go/internal/platform/qbo"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
	"github.com/tonypk/aistarlight-go/pkg/pagination"
)

var (
	ErrQBONotConnected = errors.New("no active QBO connection")
	ErrQBOTokenExpired = errors.New("QBO refresh token expired; reconnect required")
)

type QBOService struct {
	q         *sqlc.Queries
	oauth     *qbo.OAuthProvider
	client    *qbo.Client
	encryptor *crypto.AESEncryptor
}

func NewQBOService(q *sqlc.Queries, oauth *qbo.OAuthProvider, client *qbo.Client, encryptor *crypto.AESEncryptor) *QBOService {
	return &QBOService{q: q, oauth: oauth, client: client, encryptor: encryptor}
}

// AuthURL generates the QBO OAuth authorization URL.
func (s *QBOService) AuthURL(companyID uuid.UUID) string {
	return s.oauth.AuthURL(companyID.String())
}

// HandleCallback exchanges the auth code for tokens and stores the connection.
func (s *QBOService) HandleCallback(ctx context.Context, companyID uuid.UUID, code, realmID string) (*domain.QBOConnection, error) {
	tokenResp, err := s.oauth.ExchangeCode(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("exchange code: %w", err)
	}

	accessEnc, err := s.encryptor.EncryptString(tokenResp.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("encrypt access token: %w", err)
	}
	refreshEnc, err := s.encryptor.EncryptString(tokenResp.RefreshToken)
	if err != nil {
		return nil, fmt.Errorf("encrypt refresh token: %w", err)
	}

	tokenExpiry := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	refreshExpiry := time.Now().Add(time.Duration(tokenResp.RefreshExpiresIn) * time.Second)

	// Deactivate existing connection if any
	existing, err := s.q.GetQBOConnectionByCompany(ctx, companyID)
	if err == nil {
		_ = s.q.DeactivateQBOConnection(ctx, existing.ID)
	}

	scope := "com.intuit.quickbooks.accounting"
	dbConn, err := s.q.CreateQBOConnection(ctx, sqlc.CreateQBOConnectionParams{
		ID:              uuid.New(),
		CompanyID:       companyID,
		RealmID:         realmID,
		AccessTokenEnc:  accessEnc,
		RefreshTokenEnc: refreshEnc,
		TokenExpiry:     tokenExpiry,
		RefreshExpiry:   refreshExpiry,
		Scope:           &scope,
	})
	if err != nil {
		return nil, fmt.Errorf("store connection: %w", err)
	}

	return toQBOConnection(dbConn), nil
}

// Status returns the current QBO connection status (no tokens exposed).
func (s *QBOService) Status(ctx context.Context, companyID uuid.UUID) (*domain.QBOConnection, error) {
	dbConn, err := s.q.GetQBOConnectionByCompany(ctx, companyID)
	if err != nil {
		return nil, ErrQBONotConnected
	}
	return toQBOConnection(dbConn), nil
}

// Disconnect deactivates the QBO connection.
func (s *QBOService) Disconnect(ctx context.Context, companyID uuid.UUID) error {
	dbConn, err := s.q.GetQBOConnectionByCompany(ctx, companyID)
	if err != nil {
		return ErrQBONotConnected
	}
	return s.q.DeactivateQBOConnection(ctx, dbConn.ID)
}

// SyncAccounts pulls Chart of Accounts from QBO and upserts into local accounts table.
func (s *QBOService) SyncAccounts(ctx context.Context, companyID uuid.UUID, accountSvc *AccountService) (*domain.QBOSyncLog, error) {
	conn, accessToken, err := s.getConnection(ctx, companyID)
	if err != nil {
		return nil, err
	}

	// Create sync log
	logEntry, err := s.q.CreateQBOSyncLog(ctx, sqlc.CreateQBOSyncLogParams{
		ID:            uuid.New(),
		ConnectionID:  conn.ID,
		CompanyID:     companyID,
		EntityType:    "accounts",
		SyncType:      "full",
		SyncDirection: "pull",
	})
	if err != nil {
		return nil, fmt.Errorf("create sync log: %w", err)
	}

	// Query all accounts from QBO
	resp, err := s.client.Query(ctx, conn.RealmID, accessToken, "SELECT * FROM Account MAXRESULTS 1000")
	if err != nil {
		s.completeSyncLog(ctx, logEntry.ID, 0, 0, err.Error(), "failed")
		return nil, fmt.Errorf("query QBO accounts: %w", err)
	}

	synced := 0
	failed := 0
	for _, qboAcct := range resp.QueryResponse.Account {
		acctType, normalBal := mapQBOAccountType(qboAcct.Classification)

		// Check if already linked
		existing, existErr := s.q.GetAccountByQBOID(ctx, sqlc.GetAccountByQBOIDParams{
			CompanyID:    companyID,
			QboAccountID: &qboAcct.ID,
		})
		if existErr == nil {
			// Update existing
			err := accountSvc.Update(ctx, existing.ID, &qboAcct.Name, nil, nil, nil, &qboAcct.ID)
			if err != nil {
				failed++
				slog.Warn("update QBO-linked account", "qbo_id", qboAcct.ID, "error", err)
			} else {
				synced++
			}
			continue
		}

		// Create new
		acctNum := qboAcct.AcctNum
		if acctNum == "" {
			acctNum = fmt.Sprintf("QBO-%s", qboAcct.ID)
		}

		_, err := accountSvc.Create(ctx, CreateAccountInput{
			CompanyID:     companyID,
			AccountNumber: acctNum,
			Name:          qboAcct.Name,
			AccountType:   acctType,
			NormalBalance: normalBal,
			QBOAccountID:  &qboAcct.ID,
		})
		if err != nil {
			failed++
			slog.Warn("create QBO account", "qbo_id", qboAcct.ID, "error", err)
		} else {
			synced++
		}
	}

	s.completeSyncLog(ctx, logEntry.ID, synced, failed, "", "completed")
	_ = s.q.UpdateQBOSyncStatus(ctx, sqlc.UpdateQBOSyncStatusParams{
		ID:             conn.ID,
		LastSyncStatus: strPtr("completed"),
	})

	return s.getSyncLog(ctx, logEntry.ID)
}

// ListSyncLogs returns paginated sync logs.
func (s *QBOService) ListSyncLogs(ctx context.Context, companyID uuid.UUID, p pagination.Params) ([]domain.QBOSyncLog, int64, error) {
	logs, err := s.q.ListQBOSyncLogs(ctx, sqlc.ListQBOSyncLogsParams{
		CompanyID: companyID,
		Limit:     int32(p.Limit),
		Offset:    int32(p.Offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list sync logs: %w", err)
	}

	count, err := s.q.CountQBOSyncLogs(ctx, companyID)
	if err != nil {
		return nil, 0, fmt.Errorf("count sync logs: %w", err)
	}

	result := make([]domain.QBOSyncLog, len(logs))
	for i, l := range logs {
		result[i] = *toSyncLog(l)
	}
	return result, count, nil
}

// getConnection retrieves the active connection and decrypts the access token.
// Automatically refreshes if expired.
func (s *QBOService) getConnection(ctx context.Context, companyID uuid.UUID) (*sqlc.QboConnection, string, error) {
	conn, err := s.q.GetQBOConnectionByCompany(ctx, companyID)
	if err != nil {
		return nil, "", ErrQBONotConnected
	}

	// Check if refresh token expired
	if time.Now().After(conn.RefreshExpiry) {
		return nil, "", ErrQBOTokenExpired
	}

	// Refresh access token if expired (or about to expire in 5 minutes)
	if time.Now().Add(5 * time.Minute).After(conn.TokenExpiry) {
		refreshToken, err := s.encryptor.DecryptString(conn.RefreshTokenEnc)
		if err != nil {
			return nil, "", fmt.Errorf("decrypt refresh token: %w", err)
		}

		tokenResp, err := s.oauth.RefreshTokens(ctx, refreshToken)
		if err != nil {
			return nil, "", fmt.Errorf("refresh tokens: %w", err)
		}

		accessEnc, _ := s.encryptor.EncryptString(tokenResp.AccessToken)
		refreshEnc, _ := s.encryptor.EncryptString(tokenResp.RefreshToken)

		_ = s.q.UpdateQBOTokens(ctx, sqlc.UpdateQBOTokensParams{
			ID:              conn.ID,
			AccessTokenEnc:  accessEnc,
			RefreshTokenEnc: refreshEnc,
			TokenExpiry:     time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
			RefreshExpiry:   time.Now().Add(time.Duration(tokenResp.RefreshExpiresIn) * time.Second),
		})

		return &conn, tokenResp.AccessToken, nil
	}

	accessToken, err := s.encryptor.DecryptString(conn.AccessTokenEnc)
	if err != nil {
		return nil, "", fmt.Errorf("decrypt access token: %w", err)
	}

	return &conn, accessToken, nil
}

func (s *QBOService) completeSyncLog(ctx context.Context, logID uuid.UUID, synced, failed int, errMsg, status string) {
	var errDetails []byte
	if errMsg != "" {
		errDetails = []byte(fmt.Sprintf(`{"error":"%s"}`, errMsg))
	}
	_ = s.q.CompleteQBOSyncLog(ctx, sqlc.CompleteQBOSyncLogParams{
		ID:            logID,
		RecordsSynced: int32(synced),
		RecordsFailed: int32(failed),
		ErrorDetails:  errDetails,
		Status:        status,
	})
}

func (s *QBOService) getSyncLog(ctx context.Context, id uuid.UUID) (*domain.QBOSyncLog, error) {
	logs, err := s.q.ListQBOSyncLogs(ctx, sqlc.ListQBOSyncLogsParams{
		CompanyID: uuid.Nil, // not used for single fetch
		Limit:     1,
		Offset:    0,
	})
	if err != nil || len(logs) == 0 {
		return nil, fmt.Errorf("get sync log: %w", err)
	}
	return toSyncLog(logs[0]), nil
}

func mapQBOAccountType(classification string) (domain.AccountType, domain.NormalBalance) {
	switch classification {
	case "Asset":
		return domain.AccountTypeAsset, domain.NormalDebit
	case "Liability":
		return domain.AccountTypeLiability, domain.NormalCredit
	case "Equity":
		return domain.AccountTypeEquity, domain.NormalCredit
	case "Revenue":
		return domain.AccountTypeRevenue, domain.NormalCredit
	case "Expense":
		return domain.AccountTypeExpense, domain.NormalDebit
	default:
		return domain.AccountTypeExpense, domain.NormalDebit
	}
}

func toQBOConnection(c sqlc.QboConnection) *domain.QBOConnection {
	conn := &domain.QBOConnection{
		ID:            c.ID,
		CompanyID:     c.CompanyID,
		RealmID:       c.RealmID,
		TokenExpiry:   c.TokenExpiry,
		RefreshExpiry: c.RefreshExpiry,
		Scope:         c.Scope,
		IsActive:      c.IsActive,
		CreatedAt:     c.CreatedAt,
		UpdatedAt:     c.UpdatedAt,
	}
	if c.LastSyncAt.Valid {
		t := c.LastSyncAt.Time
		conn.LastSyncAt = &t
	}
	conn.LastSyncStatus = c.LastSyncStatus
	return conn
}

func toSyncLog(l sqlc.QboSyncLog) *domain.QBOSyncLog {
	log := &domain.QBOSyncLog{
		ID:            l.ID,
		ConnectionID:  l.ConnectionID,
		CompanyID:     l.CompanyID,
		EntityType:    l.EntityType,
		SyncType:      l.SyncType,
		SyncDirection: l.SyncDirection,
		StartedAt:     l.StartedAt,
		RecordsSynced: int(l.RecordsSynced),
		RecordsFailed: int(l.RecordsFailed),
		ErrorDetails:  domain.JSON(l.ErrorDetails),
		Status:        domain.SyncStatus(l.Status),
	}
	if l.CompletedAt.Valid {
		t := l.CompletedAt.Time
		log.CompletedAt = &t
	}
	return log
}

func strPtr(s string) *string { return &s }
