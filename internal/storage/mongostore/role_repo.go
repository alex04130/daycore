package mongostore

import (
	"context"
	"time"

	"daycore/internal/domain"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// _id is the role name: it is the identifier and it is unique by nature, so the
// upsert below needs no second index to be correct — the same reasoning
// settingDoc uses.
type roleDoc struct {
	Name        string    `bson:"_id"`
	Description string    `bson:"description,omitempty"`
	Permissions []string  `bson:"permissions,omitempty"`
	CreatedAt   time.Time `bson:"created_at"`
	UpdatedAt   time.Time `bson:"updated_at"`
}

func (d roleDoc) toDomain() domain.Role {
	perms := d.Permissions
	if perms == nil {
		perms = []string{}
	}
	return domain.Role{
		Name: d.Name, Description: d.Description, Permissions: perms,
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
}

// _id is "<role>:<user>", composing the pair the SQL side spells as a two-column
// primary key. That is what makes AddMember idempotent with no unique index to
// keep in step.
type roleMemberDoc struct {
	Key      string    `bson:"_id"`
	RoleName string    `bson:"role_name"`
	UserID   string    `bson:"user_id"`
	AddedAt  time.Time `bson:"added_at"`
}

func roleMemberKey(role, user string) string { return role + ":" + user }

type roleRepo struct{ *Store }

func (r roleRepo) ListRoles(ctx context.Context) ([]domain.Role, error) {
	cur, err := r.c("roles").Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []domain.Role{}
	for cur.Next(ctx) {
		var d roleDoc
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		out = append(out, d.toDomain())
	}
	return out, cur.Err()
}

func (r roleRepo) GetRole(ctx context.Context, name string) (*domain.Role, error) {
	var d roleDoc
	err := r.c("roles").FindOne(ctx, bson.M{"_id": name}).Decode(&d)
	if err == mongo.ErrNoDocuments {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	role := d.toDomain()
	return &role, nil
}

func (r roleRepo) UpsertRole(ctx context.Context, role domain.Role) error {
	if role.Name == "" {
		return domain.ErrMissingUpsertKey
	}
	perms := role.Permissions
	if perms == nil {
		perms = []string{}
	}
	now := time.Now().UTC()
	_, err := r.c("roles").UpdateOne(ctx, bson.M{"_id": role.Name}, bson.M{
		"$set":         bson.M{"description": role.Description, "permissions": perms, "updated_at": now},
		"$setOnInsert": bson.M{"created_at": now},
	}, options.Update().SetUpsert(true))
	return err
}

// DeleteRole drops the definition and then the membership — the same order the
// SQL side uses, and for the same reason: with no transaction, a failure
// between the two must leave rows that grant nothing rather than a role whose
// membership is unknown.
func (r roleRepo) DeleteRole(ctx context.Context, name string) error {
	if _, err := r.c("roles").DeleteOne(ctx, bson.M{"_id": name}); err != nil {
		return err
	}
	_, err := r.c("role_members").DeleteMany(ctx, bson.M{"role_name": name})
	return err
}

func (r roleRepo) RolesOf(ctx context.Context, userID string) ([]string, error) {
	return r.names(ctx, bson.M{"user_id": userID}, "role_name")
}

func (r roleRepo) MembersOf(ctx context.Context, name string) ([]string, error) {
	return r.names(ctx, bson.M{"role_name": name}, "user_id")
}

func (r roleRepo) names(ctx context.Context, filter bson.M, field string) ([]string, error) {
	cur, err := r.c("role_members").Find(ctx, filter, options.Find().SetSort(bson.D{{Key: field, Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []string{}
	for cur.Next(ctx) {
		var d roleMemberDoc
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		if field == "role_name" {
			out = append(out, d.RoleName)
		} else {
			out = append(out, d.UserID)
		}
	}
	return out, cur.Err()
}

func (r roleRepo) AddMember(ctx context.Context, name, userID string) error {
	if name == "" || userID == "" {
		return domain.ErrMissingUpsertKey
	}
	now := time.Now().UTC()
	_, err := r.c("role_members").UpdateOne(ctx, bson.M{"_id": roleMemberKey(name, userID)}, bson.M{
		"$set":         bson.M{"role_name": name, "user_id": userID},
		"$setOnInsert": bson.M{"added_at": now},
	}, options.Update().SetUpsert(true))
	return err
}

func (r roleRepo) RemoveMember(ctx context.Context, name, userID string) error {
	_, err := r.c("role_members").DeleteOne(ctx, bson.M{"_id": roleMemberKey(name, userID)})
	return err
}
