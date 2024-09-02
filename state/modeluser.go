// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/internal/mongo"
)

// setModelAccess changes the user's access permissions on the model.
func (st *State) setModelAccess(access permission.Access, userGlobalKey, modelUUID string) error {
	if err := permission.ValidateModelAccess(access); err != nil {
		return errors.Trace(err)
	}
	op := updatePermissionOp(modelKey(modelUUID), userGlobalKey, access)
	err := st.db().RunTransactionFor(modelUUID, []txn.Op{op})
	if err == txn.ErrAborted {
		return errors.NotFoundf("existing permissions")
	}
	return errors.Trace(err)
}

// ModelUser a model userAccessDoc.
func (st *State) modelUser(modelUUID string, user names.UserTag) (userAccessDoc, error) {
	modelUser := userAccessDoc{}
	modelUsers, closer := st.db().GetCollectionFor(modelUUID, modelUsersC)
	defer closer()

	username := strings.ToLower(user.Id())
	err := modelUsers.FindId(username).One(&modelUser)
	if err == mgo.ErrNotFound {
		return userAccessDoc{}, errors.NotFoundf("model user %q", username)
	}
	if err != nil {
		return userAccessDoc{}, errors.Trace(err)
	}
	// DateCreated is inserted as UTC, but read out as local time. So we
	// convert it back to UTC here.
	modelUser.DateCreated = modelUser.DateCreated.UTC()
	return modelUser, nil
}

func createModelUserOps(modelUUID string, user, createdBy names.UserTag, displayName string, dateCreated time.Time, access permission.Access) []txn.Op {
	creatorname := createdBy.Id()
	doc := &userAccessDoc{
		ID:          userAccessID(user),
		ObjectUUID:  modelUUID,
		UserName:    user.Id(),
		DisplayName: displayName,
		CreatedBy:   creatorname,
		DateCreated: dateCreated,
	}

	ops := []txn.Op{
		createPermissionOp(modelKey(modelUUID), userGlobalKey(userAccessID(user)), access),
		{
			C:      modelUsersC,
			Id:     userAccessID(user),
			Assert: txn.DocMissing,
			Insert: doc,
		},
	}
	return ops
}

func removeModelUserOps(modelUUID string, user names.UserTag) []txn.Op {
	return []txn.Op{
		removePermissionOp(modelKey(modelUUID), userGlobalKey(userAccessID(user))),
		{
			C:      modelUsersC,
			Id:     userAccessID(user),
			Assert: txn.DocExists,
			Remove: true,
		}}
}

func removeModelUserOpsGlobal(modelUUID string, user names.UserTag) []txn.Op {
	return []txn.Op{
		removePermissionOp(modelKey(modelUUID), userGlobalKey(userAccessID(user))),
		{
			C:      modelUsersC,
			Id:     ensureModelUUID(modelUUID, userAccessID(user)),
			Assert: txn.DocExists,
			Remove: true,
		}}
}

// removeModelUser removes a user from the database.
func (st *State) removeModelUser(user names.UserTag) error {
	ops := removeModelUserOps(st.ModelUUID(), user)
	err := st.db().RunTransaction(ops)
	if err == txn.ErrAborted {
		err = errors.NewNotFound(nil, fmt.Sprintf("model user %q does not exist", user.Id()))
	}
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// modelsForUser gives you the information about all models that a user has access to.
// This includes the name and UUID, as well as the last time the user connected to that model.
func (st *State) modelQueryForUser(user names.UserTag, isSuperuser bool) (mongo.Query, SessionCloser, error) {
	var modelQuery mongo.Query
	models, closer := st.db().GetCollection(modelsC)
	if isSuperuser {
		// Fast path, we just return all the models that aren't Importing
		modelQuery = models.Find(bson.M{"migration-mode": bson.M{"$ne": MigrationModeImporting}})
	} else {
		// Start by looking up model uuids that the user has access to, and then load only the records that are
		// included in that set
		var modelUUID struct {
			UUID string `bson:"object-uuid"`
		}
		modelUsers, userCloser := st.db().GetRawCollection(modelUsersC)
		defer userCloser()
		query := modelUsers.Find(bson.D{{"user", user.Id()}})
		query.Select(bson.M{"object-uuid": 1, "_id": 0})
		query.Batch(100)
		iter := query.Iter()
		var modelUUIDs []string
		for iter.Next(&modelUUID) {
			modelUUIDs = append(modelUUIDs, modelUUID.UUID)
		}
		if err := iter.Close(); err != nil {
			closer()
			return nil, nil, errors.Trace(err)
		}
		modelQuery = models.Find(bson.M{
			"_id":            bson.M{"$in": modelUUIDs},
			"migration-mode": bson.M{"$ne": MigrationModeImporting},
		})
	}
	modelQuery.Sort("name", "owner")
	return modelQuery, closer, nil
}

// IsControllerAdmin returns true if the user specified has Super User Access.
func (st *State) IsControllerAdmin(user names.UserTag) (bool, error) {
	model, err := st.Model()
	if err != nil {
		return false, errors.Trace(err)
	}
	ua, err := st.UserAccess(user, model.ControllerTag())
	if errors.Is(err, errors.NotFound) {
		return false, nil
	}
	if err != nil {
		return false, errors.Trace(err)
	}
	return ua.Access == permission.SuperuserAccess, nil
}

func (st *State) isControllerOrModelAdmin(user names.UserTag) (bool, error) {
	isAdmin, err := st.IsControllerAdmin(user)
	if err != nil {
		return false, errors.Trace(err)
	}
	if isAdmin {
		return true, nil
	}
	ua, err := st.UserAccess(user, names.NewModelTag(st.ModelUUID()))
	if errors.Is(err, errors.NotFound) {
		return false, nil
	}
	if err != nil {
		return false, errors.Trace(err)
	}
	return ua.Access == permission.AdminAccess, nil
}
