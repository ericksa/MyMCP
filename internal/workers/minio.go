package workers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinIOWorker struct {
	client     *minio.Client
	bucketName string
	httpClient *http.Client
}

type MinIOConfig struct {
	Endpoint  string `json:"endpoint"`
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
	Bucket    string `json:"bucket"`
	UseSSL    bool   `json:"use_ssl"`
}

func NewMinIOWorker(config MinIOConfig) (*MinIOWorker, error) {
	client, err := minio.New(config.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(config.AccessKey, config.SecretKey, ""),
		Secure: config.UseSSL,
	})
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	exists, err := client.BucketExists(ctx, config.Bucket)
	if err != nil {
		return nil, err
	}
	if !exists {
		if err := client.MakeBucket(ctx, config.Bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, err
		}
	}

	return &MinIOWorker{
		client:     client,
		bucketName: config.Bucket,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}, nil
}

func (w *MinIOWorker) GetTools() []ToolDef {
	return []ToolDef{
		{Name: "list_buckets", Description: "List all buckets"},
		{Name: "list_objects", Description: "List objects in a bucket"},
		{Name: "upload_object", Description: "Upload object to bucket"},
		{Name: "download_object", Description: "Download object from bucket"},
		{Name: "delete_object", Description: "Delete object from bucket"},
		{Name: "presigned_url", Description: "Generate presigned URL for object"},
	}
}

func (w *MinIOWorker) Execute(ctx context.Context, name string, input json.RawMessage) ([]byte, error) {
	switch name {
	case "list_buckets", "minio_list_buckets":
		return w.listBuckets(ctx, input)
	case "list_objects", "minio_list_objects":
		return w.listObjects(ctx, input)
	case "upload_object", "minio_upload_object":
		return w.uploadObject(ctx, input)
	case "download_object", "minio_download_object":
		return w.downloadObject(ctx, input)
	case "delete_object", "minio_delete_object":
		return w.deleteObject(ctx, input)
	case "presigned_url", "minio_presigned_url":
		return w.presignedURL(ctx, input)
	default:
		return nil, nil
	}
}

type ListBucketsInput struct{}

func (w *MinIOWorker) listBuckets(ctx context.Context, input json.RawMessage) ([]byte, error) {
	buckets, err := w.client.ListBuckets(ctx)
	if err != nil {
		return nil, err
	}
	var result []map[string]interface{}
	for _, b := range buckets {
		result = append(result, map[string]interface{}{
			"name":    b.Name,
			"created": b.CreationDate,
		})
	}
	return json.Marshal(result)
}

type ListObjectsInput struct {
	Prefix string `json:"prefix"`
}

func (w *MinIOWorker) listObjects(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req ListObjectsInput
	json.Unmarshal(input, &req)

	objects := w.client.ListObjects(ctx, w.bucketName, minio.ListObjectsOptions{
		Prefix:    req.Prefix,
		Recursive: true,
	})

	var result []map[string]interface{}
	for obj := range objects {
		result = append(result, map[string]interface{}{
			"key":           obj.Key,
			"size":          obj.Size,
			"etag":          obj.ETag,
			"last_modified": obj.LastModified,
			"content_type":  obj.ContentType,
		})
	}
	return json.Marshal(result)
}

type UploadObjectInput struct {
	Key         string `json:"key"`
	Content     string `json:"content"`
	ContentType string `json:"content_type"`
}

func (w *MinIOWorker) uploadObject(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req UploadObjectInput
	json.Unmarshal(input, &req)

	if req.Key == "" {
		return nil, fmt.Errorf("key is required")
	}

	contentType := req.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	_, err := w.client.PutObject(ctx, w.bucketName, req.Key, bytes.NewReader([]byte(req.Content)), int64(len(req.Content)), minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]string{
		"status": "uploaded",
		"key":    req.Key,
	})
}

type DownloadObjectInput struct {
	Key string `json:"key"`
}

func (w *MinIOWorker) downloadObject(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req DownloadObjectInput
	json.Unmarshal(input, &req)

	if req.Key == "" {
		return nil, fmt.Errorf("key is required")
	}

	obj, err := w.client.GetObject(ctx, w.bucketName, req.Key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer obj.Close()

	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]string{
		"key":     req.Key,
		"content": string(data),
	})
}

type DeleteObjectInput struct {
	Key string `json:"key"`
}

func (w *MinIOWorker) deleteObject(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req DeleteObjectInput
	json.Unmarshal(input, &req)

	if req.Key == "" {
		return nil, fmt.Errorf("key is required")
	}

	err := w.client.RemoveObject(ctx, w.bucketName, req.Key, minio.RemoveObjectOptions{})
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]string{
		"status": "deleted",
		"key":    req.Key,
	})
}

type PresignedURLInput struct {
	Key    string `json:"key"`
	Expiry int    `json:"expiry"`
}

func (w *MinIOWorker) presignedURL(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req PresignedURLInput
	json.Unmarshal(input, &req)

	if req.Key == "" {
		return nil, fmt.Errorf("key is required")
	}

	expiry := req.Expiry
	if expiry == 0 {
		expiry = 3600
	}

	url, err := w.client.PresignedGetObject(ctx, w.bucketName, req.Key, time.Duration(expiry)*time.Second, nil)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]string{
		"url": url.String(),
	})
}
