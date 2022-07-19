package main

import (
	"crypto/rsa"
	"crypto/x509"
	_ "embed"
	"encoding/pem"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

const cookieName = "isuports_session"

var key rsa.PrivateKey

func getTenantName(domain string) string {
	return strings.Split(domain, ".")[0]
}

func getNameParam(r *http.Request) (string, error) {
	err := r.ParseForm()
	if err != nil {
		return "", err
	}
	id := r.Form.Get("id")
	if id == "" {
		return "", fmt.Errorf("id is not found")
	}
	return id, nil
}

func loginPlayerHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getNameParam(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	tenant := getTenantName(r.Host)

	token := jwt.New()
	token.Set(jwt.IssuerKey, "isuports")
	token.Set(jwt.SubjectKey, id)
	token.Set(jwt.AudienceKey, tenant)
	token.Set("role", "player")
	token.Set(jwt.ExpirationKey, time.Now().Add(24*time.Hour).Unix())

	signed, err := jwt.Sign(token, jwt.WithKey(jwa.RS256, privateKey))
	if err != nil {
		fmt.Println("error jwt.Sign: %w", err)
		return
	}

	cookie := &http.Cookie{
		Name:     cookieName,
		Value:    fmt.Sprintf("%s", signed),
		Path:     "/",
		HttpOnly: true,
	}
	http.SetCookie(w, cookie)
	w.WriteHeader(http.StatusOK)
}

func loginOrganizerHandler(w http.ResponseWriter, r *http.Request) {
	tenant := getTenantName(r.Host)

	token := jwt.New()
	token.Set(jwt.IssuerKey, "isuports")
	token.Set(jwt.SubjectKey, "organizer")
	token.Set(jwt.AudienceKey, tenant)
	token.Set("role", "organizer")
	token.Set(jwt.ExpirationKey, time.Now().Add(24*time.Hour).Unix())

	signed, err := jwt.Sign(token, jwt.WithKey(jwa.RS256, privateKey))
	if err != nil {
		fmt.Println("error jwt.Sign: %w", err)
		return
	}

	cookie := &http.Cookie{
		Name:     cookieName,
		Value:    fmt.Sprintf("%s", signed),
		Path:     "/",
		HttpOnly: true,
	}
	http.SetCookie(w, cookie)
	w.WriteHeader(http.StatusOK)
}

func loginAdminHandler(w http.ResponseWriter, r *http.Request) {
	token := jwt.New()
	token.Set(jwt.IssuerKey, "isuports")
	token.Set(jwt.SubjectKey, "admin")
	token.Set(jwt.AudienceKey, "admin")
	token.Set("role", "admin")
	token.Set(jwt.ExpirationKey, time.Now().Add(24*time.Hour).Unix())

	signed, err := jwt.Sign(token, jwt.WithKey(jwa.RS256, privateKey))
	if err != nil {
		fmt.Println("error jwt.Sign: %w", err)
		return
	}

	cookie := &http.Cookie{
		Name:     cookieName,
		Value:    fmt.Sprintf("%s", signed),
		Path:     "/",
		HttpOnly: true,
	}
	http.SetCookie(w, cookie)
	w.WriteHeader(http.StatusOK)
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	cookie := &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Expires:  time.Now().Add(-time.Hour),
	}
	http.SetCookie(w, cookie)
	w.WriteHeader(http.StatusOK)
}

var privateKey *rsa.PrivateKey

//go:embed isuports.pem
var pemBytes []byte

func init() {
	// load private key
	block, _ := pem.Decode(pemBytes)
	var err error
	privateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		log.Fatalf("failed to parse private key: %s", err)
	}
}

func main() {

	// setup handler
	http.HandleFunc("/auth/login/player", loginPlayerHandler)
	http.HandleFunc("/auth/login/organizer", loginOrganizerHandler)
	http.HandleFunc("/auth/login/admin", loginAdminHandler)
	http.HandleFunc("/auth/logout", logoutHandler)
	log.Println("starting server on :3001")
	http.ListenAndServe(":3001", nil)
}
