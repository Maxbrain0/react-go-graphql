package gql

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/graphql-go/graphql"
	"github.com/maxbrain0/react-go-graphql/server/auth"
	"github.com/maxbrain0/react-go-graphql/server/errors"
	"github.com/maxbrain0/react-go-graphql/server/models"
	uuid "github.com/satori/go.uuid"
	"github.com/sirupsen/logrus"
)

var fbClient = &http.Client{
	Timeout: time.Second * 5,
}

// FBVerificationResponse used for getting json data response for validating respons
type FBVerificationResponse struct {
	Data struct {
		IsValid             bool   `json:"is_valid"`
		AppID               string `json:"app_id"`
		UserID              string `json:"user_id"`
		DataAccessExpiresAt int    `json:"data_access_expires_at"`
		ExpiresAt           int    `json:"expires_at"`
	} `json:"data"`
}

// FBUserResponse holds profile information used for creating initial user on our site
type FBUserResponse struct {
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture struct {
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
	} `json:"picture"`
}

// GoogleIDClaims holds data from Google ID token
type GoogleIDClaims struct {
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

// googleLoginWithToken is a helper function to verify the validity of the id_token provided by Google
func googleLoginWithToken(p graphql.ResolveParams) (interface{}, error) {
	rawToken := p.Args["idToken"].(string)

	idToken, err := auth.GoogleVerifier.Verify(p.Context, rawToken)

	if err != nil {
		return nil, errors.NewAuthentication("Invalid credentials", err)
	}

	var claims GoogleIDClaims

	if err := idToken.Claims(&claims); err != nil {
		return nil, errors.NewAuthentication("Invalid credentials", err)
	}

	user := models.User{
		Email:    claims.Email,
		Name:     claims.Name,
		ImageURI: claims.Picture,
	}

	loginErr := user.Login(p)

	if loginErr != nil {
		ctxLogger.WithFields(logrus.Fields{
			"Email":   user.Email,
			"Message": loginErr,
		}).Warn("Unable to login user with Google")
		return nil, loginErr
	}

	return user, nil
}

// fbLoginWithToken is a helper function to verify the validity of the access token provided by FB
// this token is not the same as the ID token. We also verify this token with FB via and http req
func fbLoginWithToken(p graphql.ResolveParams) (interface{}, error) {
	userToken := p.Args["accessToken"].(string)
	appToken := auth.FBAccessToken

	// verify Facebook user at prescribed endpoint
	fbTokenReqURL := fmt.Sprintf("https://graph.facebook.com/debug_token?input_token=%s&access_token=%s",
		userToken,
		appToken,
	)

	respToken, err := fbClient.Get(fbTokenReqURL)

	if err != nil {
		return nil, err
	}

	var fbTokenData FBVerificationResponse

	json.NewDecoder(respToken.Body).Decode(&fbTokenData)

	ctxLogger.WithFields(logrus.Fields{
		"IsValid": fbTokenData.Data.IsValid,
	}).Debugln("Successfully verified FB access token validity")

	// make sure token is Valid
	if !fbTokenData.Data.IsValid || (os.Getenv("FACEBOOK_CLIENT_ID") != fbTokenData.Data.AppID) {
		return false, errors.NewAuthentication("Invalid credentials", err)
	}

	respToken.Body.Close()

	// verify the user
	fbUserReqURL := fmt.Sprintf("https://graph.facebook.com/v5.0/me?fields=name,email,picture{url}&access_token=%v",
		userToken,
	)

	respUser, err := fbClient.Get(fbUserReqURL)
	if err != nil {
		return nil, errors.NewAuthentication("Could not validate credentials", nil)
	}

	defer respUser.Body.Close()

	var fbUserData FBUserResponse
	json.NewDecoder(respUser.Body).Decode(&fbUserData)

	user := models.User{
		Email:    fbUserData.Email,
		Name:     fbUserData.Name,
		ImageURI: fbUserData.Picture.Data.URL,
	}

	// create jwt and send cookie
	loginErr := user.Login(p)

	if loginErr != nil {
		ctxLogger.WithFields(logrus.Fields{
			"Email":   user.Email,
			"Message": loginErr,
		}).Warn("Unable to login user")
		return nil, loginErr
	}

	return user, nil
}

func createUser(p graphql.ResolveParams) (interface{}, error) {
	u := models.User{}

	user := p.Args["user"].(map[string]interface{})

	// build user model making sure of valid type-assertions
	if name, ok := user["name"].(string); ok {
		u.Name = name
	}

	if email, ok := user["email"].(string); ok {
		u.Email = email
	}

	if imageURI, ok := user["imageUri"].(string); ok {
		u.ImageURI = imageURI
	}

	rs := []models.Role{}
	if inputRoles, ok := user["roles"].([]interface{}); ok {
		for _, r := range inputRoles {
			if rname, ok := r.(string); ok {
				rs = append(rs, *models.RoleMap[rname])
			}
		}
	}

	err := u.Create(p, rs)

	if err != nil {
		ctxLogger.WithFields(logrus.Fields{
			"Email": u.Email,
		}).Warn("Unable create user")
		return nil, err
	}

	return u, nil

}

func editUser(p graphql.ResolveParams) (interface{}, error) {
	u := models.User{}
	user := p.Args["user"].(map[string]interface{})
	id, err := uuid.FromString(user["id"].(string))
	if err != nil {
		return nil, errors.NewInternal("Not a valid UUID", err)
	}
	u.ID = id

	// we'll build a map as then we can ignore fields the user does not want to update
	m := make(map[string]interface{})
	if name, ok := user["name"].(string); ok {
		m["name"] = name
	}

	if email, ok := user["email"].(string); ok {
		m["email"] = email
	}

	if imageURI, ok := user["imageUri"].(string); ok {
		m["image_uri"] = imageURI
	}

	rs := []models.Role{}
	// determine if we need to update roles as this also requires clearing authorization cache
	updateRoles := false
	if inputRoles, ok := user["roles"].([]interface{}); ok {
		updateRoles = true
		for _, r := range inputRoles {
			if rname, ok := r.(string); ok {
				rs = append(rs, *models.RoleMap[rname])
			}
		}
	}

	err = u.Update(p, m, updateRoles, rs)

	if err != nil {
		ctxLogger.WithFields(logrus.Fields{
			"Email": u.Email,
		}).Warn("Unable to edit user")
		return nil, err
	}

	return u, nil
}

func deleteUser(p graphql.ResolveParams) (interface{}, error) {
	u := models.User{}
	id := p.Args["id"].(string)
	u.ID = uuid.FromStringOrNil(id)
	if err := u.Delete(p); err != nil {
		return nil, err
	}

	return id, nil
}

func createCategory(p graphql.ResolveParams) (interface{}, error) {
	c := models.Category{}

	category := p.Args["category"].(map[string]interface{})

	// build user model making sure of valid type-assertions
	if title, ok := category["title"].(string); ok {
		c.Title = strings.ToLower(title)
	}

	if description, ok := category["description"].(string); ok {
		c.Description = description
	}

	err := c.Create(p)

	if err != nil {
		ctxLogger.WithFields(logrus.Fields{
			"Title": c.Title,
		}).Warn("Unable create category")
		return nil, err
	}

	return c, nil
}

func editCategory(p graphql.ResolveParams) (interface{}, error) {
	c := models.Category{}
	category := p.Args["category"].(map[string]interface{})
	id, err := uuid.FromString(category["id"].(string))
	if err != nil {
		return nil, errors.NewInternal("Not a valid UUID", err)
	}
	c.ID = id

	// we'll build a map as then we can ignore fields the user does not want to update
	m := make(map[string]interface{})
	if title, ok := category["title"].(string); ok {
		m["title"] = title
	}

	if description, ok := category["description"].(string); ok {
		m["description"] = description
	}

	err = c.Update(p, m)

	if err != nil {
		ctxLogger.WithFields(logrus.Fields{
			"Title": c.Title,
		}).Warn("Unable to edit category")
		return nil, err
	}

	return c, nil
}

func deleteCategory(p graphql.ResolveParams) (interface{}, error) {
	c := models.Category{}
	id := p.Args["id"].(string)
	c.ID = uuid.FromStringOrNil(id)
	if err := c.Delete(p); err != nil {
		return nil, err
	}

	return id, nil
}

func createProduct(p graphql.ResolveParams) (interface{}, error) {
	pr := models.Product{}

	product := p.Args["product"].(map[string]interface{})

	// build user model making sure of valid type-assertions
	if name, ok := product["name"].(string); ok {
		pr.Name = name
	}

	if description, ok := product["description"].(string); ok {
		pr.Description = description
	}

	if price, ok := product["price"].(int); ok {
		pr.Price = price
	}

	if imageURI, ok := product["imageUri"].(string); ok {
		pr.ImageURI = imageURI
	}

	if location, ok := product["location"].(string); ok {
		pr.Location = location
	}

	cs := models.Categories{}
	if inputCategories, ok := product["categories"].([]interface{}); ok {
		for _, c := range inputCategories {
			cid, err := uuid.FromString(c.(string))

			if err != nil {
				return nil, errors.NewInput("Error adding categories for product", nil)
			}

			cModel := models.Category{}
			cModel.ID = cid
			cs = append(cs, cModel)
		}
	}

	err := pr.Create(p, cs)

	if err != nil {
		ctxLogger.WithFields(logrus.Fields{
			"Name": pr.Name,
		}).Warn("Unable create product")
		return nil, err
	}

	return pr, nil
}

func editProduct(p graphql.ResolveParams) (interface{}, error) {
	pr := models.Product{}
	product := p.Args["product"].(map[string]interface{})
	id, err := uuid.FromString(product["id"].(string))
	if err != nil {
		return nil, errors.NewInternal("Not a valid UUID", err)
	}
	pr.ID = id

	// we'll build a map as then we can ignore fields the user does not want to update
	m := make(map[string]interface{})
	if name, ok := product["name"].(string); ok {
		m["name"] = name
	}

	if description, ok := product["description"].(string); ok {
		m["description"] = description
	}

	if price, ok := product["price"].(int); ok {
		m["price"] = price
	}

	if imageURI, ok := product["imageUri"].(string); ok {
		m["image_uri"] = imageURI
	}

	if location, ok := product["location"].(string); ok {
		m["location"] = location
	}

	cs := models.Categories{}
	if inputCategories, ok := product["categories"].([]interface{}); ok {
		for _, c := range inputCategories {
			cid, err := uuid.FromString(c.(string))

			if err != nil {
				return nil, errors.NewInput("Error adding categories for product", nil)
			}

			cModel := models.Category{}
			cModel.ID = cid
			cs = append(cs, cModel)
		}
	}

	err = pr.Update(p, m, cs)

	if err != nil {
		ctxLogger.WithFields(logrus.Fields{
			"Name": pr.Name,
		}).Warn("Unable update product")
		return nil, err
	}

	return pr, nil
}

func deleteProduct(p graphql.ResolveParams) (interface{}, error) {
	pr := models.Product{}
	id := p.Args["id"].(string)
	pr.ID = uuid.FromStringOrNil(id)
	if err := pr.Delete(p); err != nil {
		return nil, err
	}

	return id, nil
}
