package idp

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type mockAzureCredentials struct {
	jwtToken JWTToken
	err      error
}

func (mc *mockAzureCredentials) Authenticate() (JWTToken, error) {
	return mc.jwtToken, mc.err
}

func TestAzureJwtStillValid(t *testing.T) {
	type jwtStillValidTest struct {
		name           string
		inputTime      time.Time
		expectedResult bool
		message        string
	}

	jwtStillValidTestCase1 := jwtStillValidTest{
		name:           "JWT still valid",
		inputTime:      time.Now().Add(10 * time.Second),
		expectedResult: true,
		message:        "should be true",
	}
	jwtStillValidTestCase2 := jwtStillValidTest{
		name:           "JWT is invalid",
		inputTime:      time.Now(),
		expectedResult: false,
		message:        "should be false",
	}

	for _, testCase := range []jwtStillValidTest{jwtStillValidTestCase1, jwtStillValidTestCase2} {
		t.Run(testCase.name, func(t *testing.T) {
			config := AzureClientConfig{}

			creds := AzureCredentials{
				clientConfig: config,
			}
			creds.jwtToken.expiresInTime = testCase.inputTime

			assert.Equalf(t, testCase.expectedResult, creds.jwtStillValid(), testCase.message)
		})
	}
}

func TestAzureAuthenticate(t *testing.T) {
	type authenticateTest struct {
		name                    string
		inputCode               int
		inputResBody            string
		inputExpireToken        time.Time
		helper                  ManagerHelper
		expectedFuncExitErrDiff error
		expectedCode            int
		expectedToken           string
	}
	exp := 5
	token := newTestJWT(t, exp)

	authenticateTestCase1 := authenticateTest{
		name:                    "Get Cached token",
		inputExpireToken:        time.Now().Add(30 * time.Second),
		helper:                  JsonParser{},
		expectedFuncExitErrDiff: nil,
		expectedCode:            200,
		expectedToken:           "",
	}

	authenticateTestCase2 := authenticateTest{
		name:          "Get Good JWT Response",
		inputCode:     200,
		inputResBody:  fmt.Sprintf("{\"access_token\":\"%s\",\"scope\":\"read:users\",\"expires_in\":%d,\"token_type\":\"Bearer\"}", token, exp),
		helper:        JsonParser{},
		expectedCode:  200,
		expectedToken: token,
	}

	authenticateTestCase3 := authenticateTest{
		name:                    "Get Bad Status Code",
		inputCode:               400,
		inputResBody:            "{}",
		helper:                  JsonParser{},
		expectedFuncExitErrDiff: fmt.Errorf("unable to get azure token, statusCode 400"),
		expectedCode:            200,
		expectedToken:           "",
	}

	for _, testCase := range []authenticateTest{authenticateTestCase1, authenticateTestCase2, authenticateTestCase3} {
		t.Run(testCase.name, func(t *testing.T) {

			jwtReqClient := mockHTTPClient{
				resBody: testCase.inputResBody,
				code:    testCase.inputCode,
			}
			config := AzureClientConfig{}

			creds := AzureCredentials{
				clientConfig: config,
				httpClient:   &jwtReqClient,
				helper:       testCase.helper,
			}
			creds.jwtToken.expiresInTime = testCase.inputExpireToken

			_, err := creds.Authenticate()
			if err != nil {
				if testCase.expectedFuncExitErrDiff != nil {
					assert.EqualError(t, err, testCase.expectedFuncExitErrDiff.Error(), "errors should be the same")
				} else {
					t.Fatal(err)
				}
			}

			assert.Equalf(t, testCase.expectedToken, creds.jwtToken.AccessToken, "two tokens should be the same")
		})
	}
}

func TestAzureUpdateUserAppMetadata(t *testing.T) {
	type updateUserAppMetadataTest struct {
		name                 string
		inputReqBody         string
		expectedReqBody      string
		appMetadata          AppMetadata
		statusCode           int
		helper               ManagerHelper
		managerCreds         ManagerCredentials
		assertErrFunc        assert.ErrorAssertionFunc
		assertErrFuncMessage string
	}

	appMetadata := AppMetadata{WTAccountID: "ok"}

	updateUserAppMetadataTestCase1 := updateUserAppMetadataTest{
		name:            "Bad Authentication",
		expectedReqBody: "",
		appMetadata:     appMetadata,
		statusCode:      400,
		helper:          JsonParser{},
		managerCreds: &mockAzureCredentials{
			jwtToken: JWTToken{},
			err:      fmt.Errorf("error"),
		},
		assertErrFunc:        assert.Error,
		assertErrFuncMessage: "should return error",
	}

	updateUserAppMetadataTestCase2 := updateUserAppMetadataTest{
		name:            "Bad Status Code",
		expectedReqBody: fmt.Sprintf("{\"extension__wt_account_id\":\"%s\",\"extension__wt_pending_invite\":null}", appMetadata.WTAccountID),
		appMetadata:     appMetadata,
		statusCode:      400,
		helper:          JsonParser{},
		managerCreds: &mockAzureCredentials{
			jwtToken: JWTToken{},
		},
		assertErrFunc:        assert.Error,
		assertErrFuncMessage: "should return error",
	}

	updateUserAppMetadataTestCase3 := updateUserAppMetadataTest{
		name:       "Bad Response Parsing",
		statusCode: 400,
		helper:     &mockJsonParser{marshalErrorString: "error"},
		managerCreds: &mockAzureCredentials{
			jwtToken: JWTToken{},
		},
		assertErrFunc:        assert.Error,
		assertErrFuncMessage: "should return error",
	}

	updateUserAppMetadataTestCase4 := updateUserAppMetadataTest{
		name:            "Good request",
		expectedReqBody: fmt.Sprintf("{\"extension__wt_account_id\":\"%s\",\"extension__wt_pending_invite\":null}", appMetadata.WTAccountID),
		appMetadata:     appMetadata,
		statusCode:      204,
		helper:          JsonParser{},
		managerCreds: &mockAzureCredentials{
			jwtToken: JWTToken{},
		},
		assertErrFunc:        assert.NoError,
		assertErrFuncMessage: "shouldn't return error",
	}

	invite := true
	updateUserAppMetadataTestCase5 := updateUserAppMetadataTest{
		name:            "Update Pending Invite",
		expectedReqBody: fmt.Sprintf("{\"extension__wt_account_id\":\"%s\",\"extension__wt_pending_invite\":true}", appMetadata.WTAccountID),
		appMetadata: AppMetadata{
			WTAccountID:     "ok",
			WTPendingInvite: &invite,
		},
		statusCode: 204,
		helper:     JsonParser{},
		managerCreds: &mockAzureCredentials{
			jwtToken: JWTToken{},
		},
		assertErrFunc:        assert.NoError,
		assertErrFuncMessage: "shouldn't return error",
	}

	for _, testCase := range []updateUserAppMetadataTest{updateUserAppMetadataTestCase1, updateUserAppMetadataTestCase2,
		updateUserAppMetadataTestCase3, updateUserAppMetadataTestCase4, updateUserAppMetadataTestCase5} {
		t.Run(testCase.name, func(t *testing.T) {
			reqClient := mockHTTPClient{
				resBody: testCase.inputReqBody,
				code:    testCase.statusCode,
			}

			manager := &AzureManager{
				httpClient:  &reqClient,
				credentials: testCase.managerCreds,
				helper:      testCase.helper,
			}

			err := manager.UpdateUserAppMetadata("1", testCase.appMetadata)
			testCase.assertErrFunc(t, err, testCase.assertErrFuncMessage)

			assert.Equal(t, testCase.expectedReqBody, reqClient.reqBody, "request body should match")
		})
	}
}

func TestAzureProfile(t *testing.T) {
	type azureProfileTest struct {
		name             string
		clientID         string
		invite           bool
		inputProfile     azureProfile
		expectedUserData UserData
	}

	azureProfileTestCase1 := azureProfileTest{
		name:     "Good Request",
		clientID: "25d0b095-0484-40d2-9fd3-03f8f4abbb3c",
		invite:   false,
		inputProfile: azureProfile{
			"id":                "test1",
			"displayName":       "John Doe",
			"userPrincipalName": "test1@test.com",
			"extension_25d0b095048440d29fd303f8f4abbb3c_wt_account_id":     "1",
			"extension_25d0b095048440d29fd303f8f4abbb3c_wt_pending_invite": false,
		},
		expectedUserData: UserData{
			Email: "test1@test.com",
			Name:  "John Doe",
			ID:    "test1",
			AppMetadata: AppMetadata{
				WTAccountID: "1",
			},
		},
	}

	azureProfileTestCase2 := azureProfileTest{
		name:     "Missing User ID",
		clientID: "25d0b095-0484-40d2-9fd3-03f8f4abbb3c",
		invite:   true,
		inputProfile: azureProfile{
			"displayName":       "John Doe",
			"userPrincipalName": "test2@test.com",
			"extension_25d0b095048440d29fd303f8f4abbb3c_wt_account_id":     "1",
			"extension_25d0b095048440d29fd303f8f4abbb3c_wt_pending_invite": true,
		},
		expectedUserData: UserData{
			Email: "test2@test.com",
			Name:  "John Doe",
			AppMetadata: AppMetadata{
				WTAccountID: "1",
			},
		},
	}

	azureProfileTestCase3 := azureProfileTest{
		name:     "Missing User Name",
		clientID: "25d0b095-0484-40d2-9fd3-03f8f4abbb3c",
		invite:   false,
		inputProfile: azureProfile{
			"id":                "test3",
			"userPrincipalName": "test3@test.com",
			"extension_25d0b095048440d29fd303f8f4abbb3c_wt_account_id":     "1",
			"extension_25d0b095048440d29fd303f8f4abbb3c_wt_pending_invite": false,
		},
		expectedUserData: UserData{
			ID:    "test3",
			Email: "test3@test.com",
			AppMetadata: AppMetadata{
				WTAccountID: "1",
			},
		},
	}

	azureProfileTestCase4 := azureProfileTest{
		name:     "Missing Extension Fields",
		clientID: "25d0b095-0484-40d2-9fd3-03f8f4abbb3c",
		invite:   false,
		inputProfile: azureProfile{
			"id":                "test4",
			"displayName":       "John Doe",
			"userPrincipalName": "test4@test.com",
		},
		expectedUserData: UserData{
			ID:          "test4",
			Name:        "John Doe",
			Email:       "test4@test.com",
			AppMetadata: AppMetadata{},
		},
	}

	for _, testCase := range []azureProfileTest{azureProfileTestCase1, azureProfileTestCase2, azureProfileTestCase3, azureProfileTestCase4} {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.expectedUserData.AppMetadata.WTPendingInvite = &testCase.invite
			userData := testCase.inputProfile.userData(testCase.clientID)

			assert.Equal(t, testCase.expectedUserData.ID, userData.ID, "User id should match")
			assert.Equal(t, testCase.expectedUserData.Email, userData.Email, "User email should match")
			assert.Equal(t, testCase.expectedUserData.Name, userData.Name, "User name should match")
			assert.Equal(t, testCase.expectedUserData.AppMetadata.WTAccountID, userData.AppMetadata.WTAccountID, "Account id should match")
			assert.Equal(t, testCase.expectedUserData.AppMetadata.WTPendingInvite, userData.AppMetadata.WTPendingInvite, "Pending invite should match")
		})
	}
}
