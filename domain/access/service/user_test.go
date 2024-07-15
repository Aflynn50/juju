// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	jujuerrors "github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	"golang.org/x/crypto/nacl/secretbox"
	gc "gopkg.in/check.v1"

	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	usererrors "github.com/juju/juju/domain/access/errors"
	usertesting "github.com/juju/juju/domain/access/testing"
	"github.com/juju/juju/internal/auth"
	jujutesting "github.com/juju/juju/testing"
)

type userServiceSuite struct {
	state *MockUserState
}

var _ = gc.Suite(&userServiceSuite{})

func (s *userServiceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockUserState(ctrl)
	return ctrl
}

func (s *userServiceSuite) service() *UserService {
	return NewUserService(s.state)
}

// TestAddUserNameNotValid is testing that if we try and add a user with a
// username that is not valid we get an error that satisfies
// usererrors.UserNameNotValid back.
func (s *userServiceSuite) TestAddUserNameNotValid(c *gc.C) {
	_, _, err := s.service().AddUser(context.Background(), AddUserArg{Name: usertesting.InvalidUsernames[0]})
	c.Assert(err, jc.ErrorIs, usererrors.UserNameNotValid)
}

// TestAddUserAlreadyExists is testing that we cannot add a user with a username
// that already exists and is active. We expect that in this case we should
// receive an error back that satisfies usererrors.AlreadyExists.
func (s *userServiceSuite) TestAddUserAlreadyExists(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// The matcher used below verifies that we generated a
	// UUID when one was not suppied in the AddUserArg.
	a := gomock.Any()
	s.state.EXPECT().AddUserWithActivationKey(a, stringerNotEmpty{}, a, a, a, a, a).Return(usererrors.UserAlreadyExists)

	_, _, err := s.service().AddUser(context.Background(), AddUserArg{
		Name:        "valid",
		CreatorUUID: newUUID(c),
		Permission: permission.AccessSpec{
			Access: permission.LoginAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        jujutesting.ControllerTag.Id(),
			},
		},
	})
	c.Assert(err, jc.ErrorIs, usererrors.UserAlreadyExists)
}

func (s *userServiceSuite) TestAddUserCreatorUUIDNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// The matcher used below verifies that we generated a
	// UUID when one was not supplied in the AddUserArg.
	a := gomock.Any()
	s.state.EXPECT().AddUserWithActivationKey(a, stringerNotEmpty{}, a, a, a, a, a).Return(usererrors.UserCreatorUUIDNotFound)

	_, _, err := s.service().AddUser(context.Background(), AddUserArg{
		Name:        "valid",
		CreatorUUID: newUUID(c),
		Permission: permission.AccessSpec{
			Access: permission.LoginAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        jujutesting.ControllerTag.Id(),
			},
		},
	})
	c.Assert(err, jc.ErrorIs, usererrors.UserCreatorUUIDNotFound)
}

// TestAddUserWithPassword is testing the happy path of addUserWithPassword.
func (s *userServiceSuite) TestAddUserWithPassword(c *gc.C) {
	defer s.setupMocks(c).Finish()

	userUUID := newUUID(c)
	creatorUUID := newUUID(c)

	perms := permission.AccessSpec{
		Access: permission.LoginAccess,
		Target: permission.ID{
			ObjectType: permission.Controller,
			Key:        jujutesting.ControllerTag.Id(),
		},
	}

	s.state.EXPECT().AddUserWithPasswordHash(
		gomock.Any(), userUUID, "valid", "display", creatorUUID, perms, gomock.Any(), gomock.Any()).Return(nil)

	pass := auth.NewPassword("password")

	_, _, err := s.service().AddUser(context.Background(), AddUserArg{
		UUID:        userUUID,
		Name:        "valid",
		DisplayName: "display",
		Password:    &pass,
		CreatorUUID: creatorUUID,
		Permission:  perms,
	})
	c.Assert(err, jc.ErrorIsNil)
}

// TestAddUserWithPasswordNotValid is checking that if we try and add a user
// with password that is not valid we get back a error that satisfies
// internal/auth.ErrPasswordNotValid.
func (s *userServiceSuite) TestAddUserWithPasswordNotValid(c *gc.C) {
	// This exceeds the maximum password length.
	buff := make([]byte, 2000)
	_, _ = rand.Read(buff)
	badPass := auth.NewPassword(base64.StdEncoding.EncodeToString(buff))

	userUUID := newUUID(c)
	creatorUUID := newUUID(c)

	_, _, err := s.service().AddUser(context.Background(), AddUserArg{
		UUID:        userUUID,
		Name:        "valid",
		DisplayName: "display",
		Password:    &badPass,
		CreatorUUID: creatorUUID,
		Permission: permission.AccessSpec{
			Access: permission.LoginAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        jujutesting.ControllerTag.Id(),
			},
		},
	})
	c.Assert(err, jc.ErrorIs, auth.ErrPasswordNotValid)
}

// TestAddUserWithPermissionInvalid is checking that if we try and
// add a user with invalid permissions that is not valid we get back a error
// that satisfies domain/user/errors.ErrPermissionNotValid.
func (s *userServiceSuite) TestAddUserWithPermissionInvalid(c *gc.C) {
	userUUID := newUUID(c)
	creatorUUID := newUUID(c)

	pass := auth.NewPassword("password")

	_, _, err := s.service().AddUser(context.Background(), AddUserArg{
		UUID:        userUUID,
		Name:        "valid",
		DisplayName: "display",
		Password:    &pass,
		CreatorUUID: creatorUUID,
	})
	c.Assert(err, jc.ErrorIs, usererrors.PermissionNotValid)
}

// TestRemoveUser is testing the happy path for removing a user.
func (s *userServiceSuite) TestRemoveUser(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().RemoveUser(gomock.Any(), "user").Return(nil)

	err := s.service().RemoveUser(context.Background(), "user")
	c.Assert(err, jc.ErrorIsNil)
}

// TestRemoveUserInvalidUsername is testing that if we supply RemoveUser with
// invalid usernames we get back an error.
func (s *userServiceSuite) TestRemoveUserInvalidUsername(c *gc.C) {
	err := s.service().RemoveUser(context.Background(), usertesting.InvalidUsernames[0])
	c.Assert(err, jc.ErrorIs, usererrors.UserNameNotValid)
}

// TestRemoveUserNotFound is testing that trying to remove a user that does not
// exist results in an error that satisfies usererrors.NotFound. We also
// check that no state changes occur.
func (s *userServiceSuite) TestRemoveUserNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().RemoveUser(gomock.Any(), "missing").Return(usererrors.UserNotFound)

	err := s.service().RemoveUser(context.Background(), "missing")
	c.Assert(err, jc.ErrorIs, usererrors.UserNotFound)
}

// TestSetPassword is testing the happy path for SetPassword.
func (s *userServiceSuite) TestSetPassword(c *gc.C) {
	defer s.setupMocks(c).Finish()

	a := gomock.Any()
	s.state.EXPECT().SetPasswordHash(a, a, a, a).Return(nil)

	err := s.service().SetPassword(context.Background(), "user", auth.NewPassword("password"))
	c.Assert(err, jc.ErrorIsNil)
}

// TestSetPasswordInvalidUsername is testing that if we supply SetPassword with
// invalid usernames we get back an error.
func (s *userServiceSuite) TestSetPasswordInvalidUsername(c *gc.C) {
	err := s.service().SetPassword(context.Background(), usertesting.InvalidUsernames[0], auth.NewPassword("password"))
	c.Assert(err, jc.ErrorIs, usererrors.UserNameNotValid)
}

// TestSetPasswordUserNotFound is testing that when setting a password for a
// user that doesn't exist we get a user.NotFound error.
func (s *userServiceSuite) TestSetPasswordUserNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	a := gomock.Any()
	s.state.EXPECT().SetPasswordHash(a, a, a, a).Return(usererrors.UserNotFound)

	err := s.service().SetPassword(context.Background(), "user", auth.NewPassword("password"))
	c.Assert(err, jc.ErrorIs, usererrors.UserNotFound)
}

// TestSetPasswordInvalid is asserting that if pass invalid passwords to
// SetPassword the correct errors are returned.
func (s *userServiceSuite) TestSetPasswordInvalid(c *gc.C) {
	err := s.service().SetPassword(context.Background(), "username", auth.NewPassword(""))
	c.Assert(err, jc.ErrorIs, auth.ErrPasswordNotValid)
}

// TestResetPassword tests the happy path for resetting a user's password.
func (s *userServiceSuite) TestResetPassword(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().SetActivationKey(gomock.Any(), "name", gomock.Any()).Return(nil)

	key, err := s.service().ResetPassword(context.Background(), "name")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(key, gc.Not(gc.Equals), "")
}

// TestResetPasswordInvalidUsername is testing that if we supply ResetPassword
// with invalid usernames we get back an error.
func (s *userServiceSuite) TestResetPasswordInvalidUsername(c *gc.C) {
	_, err := s.service().ResetPassword(context.Background(), usertesting.InvalidUsernames[0])
	c.Assert(err, jc.ErrorIs, usererrors.UserNameNotValid)
}

// TestResetPassword is testing that resting a password for a user that doesn't
// exist returns a usererrors.NotFound error and that no state change occurs.
func (s *userServiceSuite) TestResetPasswordUserNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().SetActivationKey(gomock.Any(), "name", gomock.Any()).Return(usererrors.UserNotFound)

	_, err := s.service().ResetPassword(context.Background(), "name")
	c.Assert(err, jc.ErrorIs, usererrors.UserNotFound)
}

// TestGetUserNotFound is testing what the service does when we ask for a user
// that doesn't exist. The expected behaviour is that an error is returned that
// satisfies usererrors.NotFound.
func (s *userServiceSuite) TestGetUserNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := newUUID(c)
	s.state.EXPECT().GetUser(gomock.Any(), uuid).Return(user.User{}, usererrors.UserNotFound)

	_, err := s.service().GetUser(context.Background(), uuid)
	c.Assert(err, jc.ErrorIs, usererrors.UserNotFound)
}

// TestGetUser is asserting the safe path of GetUser in that if we supply a
// happy and good username and the username exists in state we get back a valid
// user object.
func (s *userServiceSuite) TestGetUser(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := newUUID(c)
	s.state.EXPECT().GetUser(gomock.Any(), uuid).Return(user.User{
		UUID: uuid,
		Name: "user",
	}, nil)

	u, err := s.service().GetUser(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(u.Name, gc.Equals, "user")
}

// TestGetUserByName tests the happy path for GetUserByName.
func (s *userServiceSuite) TestGetUserByName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := newUUID(c)
	s.state.EXPECT().GetUserByName(gomock.Any(), "name").Return(user.User{
		UUID: uuid,
		Name: "user",
	}, nil)

	u, err := s.service().GetUserByName(context.Background(), "name")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(u.UUID, gc.Equals, uuid)
}

// TestGetUserByNameNotFound is testing that if we ask for a user by name that
// doesn't exist we get back an error that satisfies usererrors.NotFound.
func (s *userServiceSuite) TestGetUserByNameNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetUserByName(gomock.Any(), "user").Return(user.User{}, usererrors.UserNotFound)

	_, err := s.service().GetUserByName(context.Background(), "user")
	c.Assert(err, jc.ErrorIs, usererrors.UserNotFound)
}

// TestGetAllUsers tests the happy path for GetAllUsers.
func (s *userServiceSuite) TestGetAllUsers(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetAllUsers(gomock.Any(), true).Return([]user.User{
		{
			UUID: newUUID(c),
			Name: "user0",
		},
		{
			UUID: newUUID(c),
			Name: "user1",
		},
	}, nil)

	users, err := s.service().GetAllUsers(context.Background(), true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(users, gc.HasLen, 2)
	c.Check(users[0].Name, gc.Equals, "user0")
	c.Check(users[1].Name, gc.Equals, "user1")
}

// TestGetUserByNameInvalidUsername is here to assert that when we ask for a user with
// a username that is invalid we get a UsernameNotValid error. We also check
// here that the service doesn't let invalid usernames flow through to the state
// layer.
func (s *userServiceSuite) TestGetUserByNameInvalidUsername(c *gc.C) {
	_, err := s.service().GetUserByName(context.Background(), usertesting.InvalidUsernames[0])
	c.Assert(err, jc.ErrorIs, usererrors.UserNameNotValid)
}

// TestGetUserByAuth is testing the happy path for GetUserByAuth.
func (s *userServiceSuite) TestGetUserByAuth(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := newUUID(c)
	s.state.EXPECT().GetUserByAuth(gomock.Any(), "name", auth.NewPassword("pass")).Return(user.User{
		UUID: uuid,
		Name: "user",
	}, nil)

	u, err := s.service().GetUserByAuth(context.Background(), "name", auth.NewPassword("pass"))
	c.Assert(err, jc.ErrorIsNil)
	c.Check(u.UUID, gc.Equals, uuid)
}

// TestEnableUserAuthentication tests the happy path for EnableUserAuthentication.
func (s *userServiceSuite) TestEnableUserAuthentication(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().EnableUserAuthentication(gomock.Any(), "name")

	err := s.service().EnableUserAuthentication(context.Background(), "name")
	c.Assert(err, jc.ErrorIsNil)
}

// TestDisableUserAuthentication tests the happy path for DisableUserAuthentication.
func (s *userServiceSuite) TestDisableUserAuthentication(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().DisableUserAuthentication(gomock.Any(), "name")

	err := s.service().DisableUserAuthentication(context.Background(), "name")
	c.Assert(err, jc.ErrorIsNil)
}

// TestSetPasswordWithActivationKey tests setting a password with an activation
// key.
func (s *userServiceSuite) TestSetPasswordWithActivationKey(c *gc.C) {
	defer s.setupMocks(c).Finish()

	password := "password123"

	// Create a key for the activation box.
	key := make([]byte, activationKeyLength)
	_, err := rand.Read(key)
	c.Assert(err, jc.ErrorIsNil)

	s.state.EXPECT().GetActivationKey(gomock.Any(), "name").Return(key, nil)
	s.state.EXPECT().SetPasswordHash(gomock.Any(), "name", gomock.Any(), gomock.Any()).Return(nil)

	// Create a nonce for the activation box.
	nonce := make([]byte, activationBoxNonceLength)
	_, err = rand.Read(nonce)
	c.Assert(err, jc.ErrorIsNil)

	type payload struct {
		Password string `json:"password"`
	}
	p := payload{
		Password: password,
	}
	payloadBytes, err := json.Marshal(p)
	c.Assert(err, jc.ErrorIsNil)

	box := s.sealBox(key, nonce, payloadBytes)

	_, err = s.service().SetPasswordWithActivationKey(context.Background(), "name", nonce, box)
	c.Assert(err, jc.ErrorIsNil)
}

// TestSetPasswordWithActivationKeyWithInvalidKey tests setting a password
// with an invalid activation key.
func (s *userServiceSuite) TestSetPasswordWithActivationKeyWithInvalidKey(c *gc.C) {
	defer s.setupMocks(c).Finish()

	password := "password123"

	// Create a key for the activation box.
	key := make([]byte, activationKeyLength)
	_, err := rand.Read(key)
	c.Assert(err, jc.ErrorIsNil)

	s.state.EXPECT().GetActivationKey(gomock.Any(), "name").Return(key, nil)

	// Create a nonce for the activation box.
	nonce := make([]byte, activationBoxNonceLength)
	_, err = rand.Read(nonce)
	c.Assert(err, jc.ErrorIsNil)

	type payload struct {
		Password string `json:"password"`
	}
	p := payload{
		Password: password,
	}
	payloadBytes, err := json.Marshal(p)
	c.Assert(err, jc.ErrorIsNil)

	box := s.sealBox(key, nonce, payloadBytes)

	// Replace the nonce with a different nonce.
	_, err = rand.Read(nonce)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.service().SetPasswordWithActivationKey(context.Background(), "name", nonce, box)
	c.Assert(err, jc.ErrorIs, usererrors.ActivationKeyNotValid)
}

func (s *userServiceSuite) sealBox(key, nonce, payload []byte) []byte {
	var sbKey [activationKeyLength]byte
	var sbNonce [activationBoxNonceLength]byte
	copy(sbKey[:], key)
	copy(sbNonce[:], nonce)

	return secretbox.Seal(nil, payload, &sbNonce, &sbKey)
}

// FuzzGetUser is a fuzz test for GetUser() that stresses the username input of
// the function to make sure that no panics occur and all input is handled
// gracefully.
func FuzzGetUser(f *testing.F) {
	for _, valid := range usertesting.ValidUsernames {
		f.Add(valid)
	}

	f.Fuzz(func(t *testing.T, username string) {
		ctrl := gomock.NewController(t)
		state := NewMockUserState(ctrl)
		defer ctrl.Finish()

		state.EXPECT().GetUserByName(gomock.Any(), username).Return(
			user.User{
				Name: username,
			},
			nil,
		).AnyTimes()

		usr, err := NewUserService(state).GetUserByName(context.Background(), username)
		if err != nil && !errors.Is(err, usererrors.UserNameNotValid) {
			t.Errorf("unexpected error %v when fuzzing GetUser with %q",
				err, username,
			)
		} else if errors.Is(err, usererrors.UserNameNotValid) {
			return
		}

		if usr.Name != username {
			t.Errorf("GetUser() user.name %q != %q", usr.Name, username)
		}
	})
}

// TestUpdateLastModelLogin tests the happy path for UpdateLastModelLogin.
func (s *userServiceSuite) TestUpdateLastModelLogin(c *gc.C) {
	defer s.setupMocks(c).Finish()
	modelUUID := modeltesting.GenModelUUID(c)
	s.state.EXPECT().UpdateLastModelLogin(gomock.Any(), "name", modelUUID)

	err := s.service().UpdateLastModelLogin(context.Background(), "name", modelUUID)
	c.Assert(err, jc.ErrorIsNil)
}

// TestUpdateLastModelLogin tests a bad username for UpdateLastModelLogin.
func (s *userServiceSuite) TestUpdateLastModelLoginBadUsername(c *gc.C) {
	defer s.setupMocks(c).Finish()
	modelUUID := modeltesting.GenModelUUID(c)
	err := s.service().UpdateLastModelLogin(context.Background(), "13987*($*($&(*&%(", modelUUID)
	c.Assert(err, jc.ErrorIs, usererrors.UserNameNotValid)
}

// TestLastModelLogin tests the happy path for LastModelLogin.
func (s *userServiceSuite) TestLastModelLogin(c *gc.C) {
	defer s.setupMocks(c).Finish()
	modelUUID := modeltesting.GenModelUUID(c)
	t := time.Now()
	s.state.EXPECT().LastModelLogin(gomock.Any(), "name", modelUUID).Return(t, nil)

	lastConnection, err := s.service().LastModelLogin(context.Background(), "name", modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(lastConnection, gc.Equals, t)
}

// TestLastModelLoginBadUsername tests a bad username for LastModelLogin.
func (s *userServiceSuite) TestLastModelLoginBadUsername(c *gc.C) {
	defer s.setupMocks(c).Finish()
	_, err := s.service().LastModelLogin(context.Background(), "1&(*£*(", "")
	c.Assert(err, jc.ErrorIs, usererrors.UserNameNotValid)
}

// TestLastModelLoginBadUUID tests a bad UUID given to LastModelLogin.
func (s *userServiceSuite) TestLastModelLoginBadUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()
	_, err := s.service().LastModelLogin(context.Background(), "name", "bad-uuid")
	c.Assert(err, jc.ErrorIs, jujuerrors.NotValid)
}

type stringerNotEmpty struct{}

func (s stringerNotEmpty) Matches(arg any) bool {
	str, ok := arg.(fmt.Stringer)
	if !ok {
		return false
	}
	return str.String() != ""
}

func (s stringerNotEmpty) String() string {
	return "matches if the input fmt.Stringer produces a non-empty string."
}
