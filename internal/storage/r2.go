// Package storage is the Cloudflare R2 client for meal photos, copied from
// journal's internal/storage/r2.go (same env var names, so credentials are
// reusable between the apps).
package storage

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type R2Client struct {
	client    *s3.Client
	bucket    string
	publicURL string
}

func NewR2Client(accountID, accessKey, secretKey, bucket, publicURL string) *R2Client {
	endpoint := fmt.Sprintf("https://%s.r2.cloudflarestorage.com", accountID)

	client := s3.New(s3.Options{
		Region:       "auto",
		BaseEndpoint: aws.String(endpoint),
		Credentials:  credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
	})

	return &R2Client{client: client, bucket: bucket, publicURL: publicURL}
}

func (r *R2Client) UploadPhoto(ctx context.Context, key, contentType string, data io.Reader) (string, error) {
	_, err := r.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(r.bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
		Body:        data,
	})
	if err != nil {
		return "", fmt.Errorf("r2 put object: %w", err)
	}
	return r.publicURL + "/" + key, nil
}

// GetPhoto fetches an object's bytes and content type, for feeding a photo
// into the vision parse loop.
func (r *R2Client) GetPhoto(ctx context.Context, key string) ([]byte, string, error) {
	out, err := r.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, "", fmt.Errorf("r2 get object: %w", err)
	}
	defer out.Body.Close()

	data, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read r2 object body: %w", err)
	}

	contentType := ""
	if out.ContentType != nil {
		contentType = *out.ContentType
	}
	return data, contentType, nil
}
