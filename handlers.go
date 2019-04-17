package hippo

import (
	"log"
	"fmt"
	"net/http"
	"database/sql"
	"html/template"
	"encoding/json"
	"net/http/httputil"
	"github.com/gin-gonic/gin"
	"github.com/nathanstitt/hippo/models"
)

func GetConfig(c *gin.Context) Configuration {
	config, ok := c.MustGet("config").(Configuration)
	if ok {
		return config
	}
	panic("config isn't the correct type")
}

func GetDB(c *gin.Context) DB {
	tx, ok := c.MustGet("dbTx").(DB)
	if ok {
		return tx
	}
	panic("config isn't the correct type")
}

func RoutingMiddleware(config Configuration, db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		tx, err := db.Begin()
		if err != nil {
			panic(err)
		}
		c.Set("dbTx", tx)
		c.Set("config", config)
		defer func() {
			status := c.Writer.Status()
			if status >= 400 {
				log.Printf("Transaction is being rolled back; status = %d\n", status)
				tx.Rollback();
			}
			return
		}()
		c.Next()
		if (c.Writer.Status() < 400) {
			tx.Commit();
		}
	}
}

func RenderErrorPage(message string, c *gin.Context, err *error) {
	if err != nil {
		log.Printf("Error occured: %s", *err)
	}
	c.HTML(http.StatusInternalServerError, "error.html", gin.H{
		"message": message,
	})
}

func RenderNotFoundPage(message string, c *gin.Context, err *error) {
	c.HTML(http.StatusNotFound , "not-found.html", gin.H{
		"message": message,
	})
}

func allowCorsReply(c *gin.Context) {
	c.Header("Content-Type", "application/json")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Max-Age", "86400")
	c.Header("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE, UPDATE")
	c.Header("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, X-Max, X-HASURA-ACCESS-KEY")
	c.Header("Access-Control-Allow-Credentials", "true")
}

func reverseProxy(port int) gin.HandlerFunc {
	target := fmt.Sprintf("localhost:%d", port)
	director := func(req *http.Request) {
		req.URL.Scheme = "http"
		req.URL.Host = target
	}
	return func(c *gin.Context) {
		proxy := &httputil.ReverseProxy{Director: director}
		proxy.ServeHTTP(c.Writer, c.Request)
	}
}

func RenderHomepage(signup *SignupData, err *error, c *gin.Context) {
	c.HTML(http.StatusOK, "home.html", gin.H{
		"signup": signup,
		"error": err,
	})
}

func RenderApplication(user *hm.User, c *gin.Context) {
	cfg := GetConfig(c)
	c.HTML(http.StatusOK, "application.html", gin.H{
		"bootstrapData": BootstrapData(user, cfg),
	})
}

func deliverResetEmail(user *hm.User, token string, db DB, config Configuration) error {
	email := NewEmailMessage(user.Tenant().OneP(db), config)
	email.Body = passwordResetEmail(user, token, db, config)
	email.SetTo(user.Email, user.Name)
	email.SetSubject("Password Reset for %s", config.String("product_name"))
	return email.Deliver()
}

func deliverLoginEmail(emailAddress string, tenant *hm.Tenant, config Configuration) error {
	email := NewEmailMessage(tenant, config)
	email.Body = signupEmail(emailAddress, tenant, config)
	email.SetTo(emailAddress, "")
	email.SetSubject("Login to %s", config.String("product_name"))
	return email.Deliver()
}

func BootstrapData(user *hm.User, cfg Configuration) template.JS {
	type BootstrapDataT map[string]interface{}
	bootstrapData, err := json.Marshal(
		BootstrapDataT{
			"serverUrl": cfg.String("server_url"),
			"user": user,
			"graphql" : BootstrapDataT{
				"token": JWTForUser(user, cfg),
				"endpoint": cfg.String("server_url"),
			},
		})
	if err != nil {
		panic(err)
	}
	return template.JS(string(bootstrapData))
}
