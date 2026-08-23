package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"time"
)

type PaymentRequest struct {
	UserID         int64       `json:"user_id"`
	Amount         json.Number `json:"amount"`
	CurrencyCode   string      `json:"currency"`
	IdempotencyKey string      `json:"-"`
}

type PaymentResponse struct {
	PaymentID string `json:"payment_id"`
	Status    string `json:"status"`
}

type ServiceError struct {
	StatusCode int    `json:"-"`
	Code       string `json:"error"`
	Message    string `json:"message"`
}

func main() {
	c := make(chan int, 1000)

	for i := 0; i < 100; i++ {
		go foo(c)

	}
	sum := 0
	for i := range c {
		sum += i

	}

	fmt.Println(sum)
}

const (
	paymentServerUrl = "http://localhost:8080"
	paymentAPI       = "/v1/api/payment"
)

type PaymentClient struct {
	httpClient       *http.Client
	maxRetryAttempts int
	baseDelay        time.Duration
}

func (client *PaymentClient) SendPaymentRequest(ctx context.Context, p PaymentRequest) (PaymentResponse, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return PaymentResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, paymentServerUrl+paymentAPI, bytes.NewReader(body))
	if err != nil {
		return PaymentResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Idempotency-key", p.IdempotencyKey)
	resp, err := client.httpClient.Do(req)
	if err != nil {
		return PaymentResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return PaymentResponse{}, errors.New(resp.Status)
	}
	var paymentResponse PaymentResponse
	err = json.NewDecoder(resp.Body).Decode(&paymentResponse)
	if err != nil {
		return PaymentResponse{}, err
	}
	return paymentResponse, nil
}

func (client *PaymentClient) SendPaymentWithRetry(ctx context.Context, req PaymentRequest) (PaymentResponse, error) {

	var lastErr error
	delay := client.baseDelay
	for attempt := 0; attempt < client.maxRetryAttempts; attempt++ {
		result, err := client.SendPaymentRequest(ctx, req)
		if err == nil {
			return result, nil
		}

		lastErr = err
		if !shouldRetry(err) {
			return result, err
		}

		jitter := time.Duration(rand.Intn(100)) * time.Millisecond
		timer := time.NewTimer(delay + jitter)

		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return PaymentResponse{}, ctx.Err()
		}
		delay = 2 * delay

	}
	return PaymentResponse{}, lastErr

}

func shouldRetry(err error) bool {
	// ...
	var networkErr net.Error
	return errors.As(err, &networkErr)
}
