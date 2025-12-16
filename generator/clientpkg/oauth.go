package clientpkg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type OAuthClient struct {
	AccessToken   string
	RefreshToken  string
	ClientId      string
	ClientSecret  string
	OAuthEndpoint string
}

func NewOAuthClient(token, refreshToken, clientId, clientSecret, endpoint string) OAuthClient {
	return OAuthClient{
		AccessToken:   token,
		RefreshToken:  refreshToken,
		ClientId:      clientId,
		ClientSecret:  clientSecret,
		OAuthEndpoint: endpoint,
	}
}

// sendRequest sends an HTTP request with the current Bearer token
func (client *OAuthClient) SendRequest(ctx context.Context, req *http.Request, depth int) *ResponseData {
	req = req.WithContext(ctx)
	// Set the Authorization header with Bearer token
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", client.AccessToken))

	// Send the request
	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return NewResponseData(nil, err)
	}

	// Check if we got a 401 with an invalid access token message
	if resp.StatusCode == http.StatusUnauthorized && depth == 0 {
		defer resp.Body.Close()
		bodyBytes, _ := io.ReadAll(req.Body)
		var respBody map[string]interface{}
		err = json.Unmarshal(bodyBytes, &respBody)
		if err != nil {
			return NewResponseData(nil, err)
		}
		if msg, ok := respBody["message"].(string); ok && msg == "invalid access_token" {
			// Token is invalid, we need to refresh
			//fmt.Println("Invalid access token, attempting to refresh token...")
			if err := client.refreshAccessToken(); err != nil {
				return NewResponseData(nil, err)
			}
			// Retry the request with the new token
			return client.SendRequest(ctx, req, depth+1)
		}
	}

	return NewResponseData(resp, nil)
}

// refreshAccessToken sends a request to the OAuth server to refresh the token
func (client *OAuthClient) refreshAccessToken() error {
	// Prepare refresh token payload
	refreshPayload := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": client.RefreshToken,
		"client_id":     client.ClientId,
		"client_secret": client.ClientSecret,
	}

	payloadBytes, err := json.Marshal(refreshPayload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", client.OAuthEndpoint, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to refresh token, status: %d", resp.StatusCode)
	}

	var respBody map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return err
	}

	// Update the access token
	if newAccessToken, ok := respBody["access_token"].(string); ok {
		client.AccessToken = newAccessToken
		fmt.Println("Access token refreshed successfully")
	} else {
		return fmt.Errorf("access token not found in refresh response")
	}

	return nil
}
