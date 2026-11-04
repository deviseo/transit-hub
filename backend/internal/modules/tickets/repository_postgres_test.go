package tickets

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"transithub/backend/internal/database/migrations"
)

// PostgreSQL 仓储测试：需要 TEST_DATABASE_URL，在临时 schema 里跑完整迁移，只写合成数据，结束后删除。
func openTicketsTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL repository tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	adminPool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect PostgreSQL: %v", err)
	}
	schema := fmt.Sprintf("tickets_test_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := adminPool.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		adminPool.Close()
		t.Fatalf("create schema: %v", err)
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		adminPool.Close()
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		adminPool.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		if _, err := adminPool.Exec(context.Background(), "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Errorf("drop schema: %v", err)
		}
		adminPool.Close()
	})
	if err := migrations.Run(ctx, pool); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	return pool
}

func TestRepositoryStoresTicketsMessagesAndAttachments(t *testing.T) {
	repo := NewRepository(openTicketsTestPool(t))
	ctx := context.Background()
	other := Workspace{UserID: "owner-2", AdminAccountID: "acct-9"}
	at := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)

	newTicket := func(id string, user SiteUser, minute int) {
		t.Helper()
		ticket := Ticket{ID: id, Workspace: shop, User: user, Contact: "c@example.com", ClientInfo: "TransitLink 0.2.0",
			Title: "标题 " + id, Category: "通用问题", Status: StatusOpen, LastAuthor: AuthorCustomer, LastMessageAt: at.Add(time.Duration(minute) * time.Minute)}
		message := Message{ID: "m-" + id, TicketID: id, AuthorType: AuthorCustomer, AuthorName: user.Username, Body: "内容 " + id,
			CreatedAt: ticket.LastMessageAt, Attachments: []Attachment{{ID: "a-" + id, OriginalName: "s.png", ContentType: "image/png", SizeBytes: 3, StoragePath: "file-" + id + ".png"}}}
		if err := repo.CreateTicket(ctx, ticket, message); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	aliceSub2 := SiteUser{Platform: "sub2api", ID: "42", Username: "alice", Email: "alice@example.com"}
	newTicket("t1", aliceSub2, 1)
	newTicket("t2", aliceSub2, 2)
	newTicket("t3", SiteUser{Platform: "newapi", ID: "42", Username: "other"}, 3)

	mine, err := repo.ListForUser(ctx, shop, aliceSub2, 10)
	if err != nil || len(mine) != 2 || mine[0].ID != "t2" || mine[0].User != aliceSub2 || mine[0].ClientInfo != "TransitLink 0.2.0" {
		t.Fatalf("list for user: %+v %v", mine, err)
	}
	if theirs, _ := repo.ListForUser(ctx, other, aliceSub2, 10); len(theirs) != 0 {
		t.Fatalf("other workspaces see nothing: %+v", theirs)
	}

	found, err := repo.AddMessage(ctx, shop, "t1", Message{ID: "m-reply", TicketID: "t1", AuthorType: AuthorAdmin, Body: "回复", CreatedAt: at.Add(time.Hour)}, StatusReplied)
	if err != nil || !found {
		t.Fatalf("add message: %v %v", found, err)
	}
	if found, _ := repo.AddMessage(ctx, other, "t1", Message{ID: "m-x", TicketID: "t1", AuthorType: AuthorAdmin, Body: "x", CreatedAt: at}, StatusReplied); found {
		t.Fatal("another workspace must not reply")
	}
	ticket, err := repo.GetTicket(ctx, shop, "t1")
	if err != nil || ticket.Status != StatusReplied || ticket.LastAuthor != AuthorAdmin || !ticket.LastMessageAt.Equal(at.Add(time.Hour)) || ticket.CustomerSeenAt != nil {
		t.Fatalf("after reply: %+v %v", ticket, err)
	}
	if err := repo.MarkCustomerSeen(ctx, shop, "t1", at.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if ticket, _ := repo.GetTicket(ctx, shop, "t1"); ticket.CustomerSeenAt == nil {
		t.Fatal("seen time not saved")
	}

	messages, err := repo.ListMessages(ctx, "t1")
	if err != nil || len(messages) != 2 || messages[0].Body != "内容 t1" || len(messages[0].Attachments) != 1 ||
		messages[0].Attachments[0].StoragePath != "file-t1.png" || len(messages[1].Attachments) != 0 {
		t.Fatalf("messages: %+v %v", messages, err)
	}
	if attachment, err := repo.GetAttachment(ctx, shop, "a-t1"); err != nil || attachment == nil || attachment.TicketID != "t1" {
		t.Fatalf("attachment: %+v %v", attachment, err)
	}
	if attachment, _ := repo.GetAttachment(ctx, other, "a-t1"); attachment != nil {
		t.Fatal("attachments are scoped to the workspace")
	}

	if found, err := repo.SetStatus(ctx, shop, "t2", StatusClosed); err != nil || !found {
		t.Fatalf("set status: %v %v", found, err)
	}
	page, total, err := repo.ListForAdmin(ctx, shop, "", 1, 2)
	if err != nil || total != 3 || len(page) != 2 || page[0].ID != "t1" {
		t.Fatalf("admin page: %+v %d %v", page, total, err)
	}
	closed, total, _ := repo.ListForAdmin(ctx, shop, StatusClosed, 1, 20)
	if total != 1 || closed[0].ID != "t2" {
		t.Fatalf("closed filter: %+v", closed)
	}
	counts, err := repo.CountByStatus(ctx, shop)
	if err != nil || counts != (StatusCounts{Open: 1, Replied: 1, Closed: 1}) {
		t.Fatalf("counts: %+v %v", counts, err)
	}
	names, err := repo.AttachmentNames(ctx, shop)
	if err != nil || len(names) != 3 {
		t.Fatalf("attachment names: %v %v", names, err)
	}
}

func TestSessionStoreInRedis(t *testing.T) {
	redisURL := strings.TrimSpace(os.Getenv("TEST_REDIS_URL"))
	if redisURL == "" {
		t.Skip("TEST_REDIS_URL is required for Redis tests")
	}
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(options)
	defer client.Close()
	ctx := context.Background()
	store := NewSessionStore(client)
	ws := Workspace{UserID: fmt.Sprintf("owner-%d", time.Now().UnixNano()), AdminAccountID: "acct-1"}
	t.Cleanup(func() { _ = store.DeleteWorkspace(ctx, ws) })

	session := ClientSession{Workspace: ws, User: SiteUser{Platform: "sub2api", ID: "42"}, ClientInfo: "x", ExpiresAt: time.Now().Add(time.Hour)}
	token, err := store.Create(ctx, session)
	if err != nil || len(token) != 64 {
		t.Fatalf("create: %q %v", token, err)
	}
	if keys, _ := client.Keys(ctx, "*"+token+"*").Result(); len(keys) != 0 {
		t.Fatalf("the raw token must not appear in Redis keys: %v", keys)
	}
	if ttl := client.TTL(ctx, sessionKeyPrefix+hashToken(token)).Val(); ttl <= 0 || ttl > time.Hour {
		t.Fatalf("session must expire: %v", ttl)
	}
	got, err := store.Get(ctx, token)
	if err != nil || got == nil || got.User.ID != "42" || got.Workspace != ws {
		t.Fatalf("get: %+v %v", got, err)
	}
	if missing, _ := store.Get(ctx, "not-a-token"); missing != nil {
		t.Fatal("unknown token")
	}
	if err := store.DeleteWorkspace(ctx, ws); err != nil {
		t.Fatal(err)
	}
	if gone, _ := store.Get(ctx, token); gone != nil {
		t.Fatal("deleting the workspace removes its sessions")
	}
}
