package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinIOWorker struct {
	client *minio.Client
	bucket string
}

type MinIOConfig struct {
	Endpoint  string `json:"endpoint"`
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
	Bucket    string `json:"bucket"`
	UseSSL    bool   `json:"use_ssl"`
}

func NewMinIOWorker(cfg MinIOConfig) (*MinIOWorker, error) {
	minioClient, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %w", err)
	}

	return &MinIOWorker{
		client: minioClient,
		bucket: cfg.Bucket,
	}, nil
}

func (w *MinIOWorker) GetTools() []ToolDef {
	return []ToolDef{
		{Name: "minio_upload_file", Description: "Upload a file to MinIO/S3"},
		{Name: "minio_download_file", Description: "Download a file from MinIO/S3"},
		{Name: "minio_list_objects", Description: "List objects in a bucket/prefix"},
		{Name: "minio_delete_object", Description: "Delete an object from MinIO/S3"},
		{Name: "minio_get_url", Description: "Get presigned URL for an object"},
		{Name: "minio_bucket_exists", Description: "Check if bucket exists"},
		{Name: "minio_make_bucket", Description: "Create a new bucket"},
		{Name: "minio_list_buckets", Description: "List all buckets"},
		{Name: "minio_get_object_info", Description: "Get object metadata"},
		{Name: "minio_copy_object", Description: "Copy object within MinIO"},
		{Name: "minio_move_object", Description: "Move/rename object in MinIO"},
		{Name: "minio_sync_directory", Description: "Sync local directory to MinIO"},
	}
}

func (w *MinIOWorker) Execute(ctx context.Context, name string, input json.RawMessage) ([]byte, error) {
	switch name {
	case "minio_upload_file":
		return w.uploadFile(ctx, input)
	case "minio_download_file":
		return w.downloadFile(ctx, input)
	case "minio_list_objects":
		return w.listObjects(ctx, input)
	case "minio_delete_object":
		return w.deleteObject(ctx, input)
	case "minio_get_url":
		return w.getPresignedURL(ctx, input)
	case "minio_bucket_exists":
		return w.bucketExists(ctx, input)
	case "minio_make_bucket":
		return w.makeBucket(ctx, input)
	case "minio_list_buckets":
		return w.listBuckets(ctx, input)
	case "minio_get_object_info":
		return w.getObjectInfo(ctx, input)
	case "minio_copy_object":
		return w.copyObject(ctx, input)
	case "minio_move_object":
		return w.moveObject(ctx, input)
	case "minio_sync_directory":
		return w.syncDirectory(ctx, input)
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

// Upload file to MinIO
func (w *MinIOWorker) uploadFile(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		LocalPath   string            `json:"local_path"`
		ObjectName  string            `json:"object_name"`
		Bucket      string            `json:"bucket,omitempty"`
		ContentType string            `json:"content_type,omitempty"`
		Metadata    map[string]string `json:"metadata,omitempty"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	bucket := req.Bucket
	if bucket == "" {
		bucket = w.bucket
	}

	// Detect content type if not provided
	contentType := req.ContentType
	if contentType == "" {
		contentType = mime.TypeByExtension(filepath.Ext(req.LocalPath))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
	}

	file, err := os.Open(req.LocalPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	uploadInfo, err := w.client.PutObject(ctx, bucket, req.ObjectName, file, stat.Size(), minio.PutObjectOptions{
		ContentType: contentType,
		UserMetadata: req.Metadata,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to upload: %w", err)
	}

	result := map[string]interface{}{
		"bucket":       bucket,
		"object_name":  req.ObjectName,
		"etag":         uploadInfo.ETag,
		"size":         uploadInfo.Size,
		"content_type": contentType,
		"metadata":     req.Metadata,
	}
	return json.Marshal(result)
}

// Download file from MinIO
func (w *MinIOWorker) downloadFile(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		ObjectName string `json:"object_name"`
		LocalPath  string `json:"local_path"`
		Bucket     string `json:"bucket,omitempty"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	bucket := req.Bucket
	if bucket == "" {
		bucket = w.bucket
	}

	object, err := w.client.GetObject(ctx, bucket, req.ObjectName, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get object: %w", err)
	}
	defer object.Close()

	// Create local directory if needed
	dir := filepath.Dir(req.LocalPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	localFile, err := os.Create(req.LocalPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create local file: %w", err)
	}
	defer localFile.Close()

	size, err := io.Copy(localFile, object)
	if err != nil {
		return nil, fmt.Errorf("failed to download: %w", err)
	}

	result := map[string]interface{}{
		"bucket":      bucket,
		"object_name": req.ObjectName,
		"local_path":  req.LocalPath,
		"size":        size,
	}
	return json.Marshal(result)
}

// List objects in bucket/prefix
func (w *MinIOWorker) listObjects(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		Bucket    string `json:"bucket,omitempty"`
		Prefix    string `json:"prefix,omitempty"`
		Recursive bool   `json:"recursive,omitempty"`
		MaxKeys   int    `json:"max_keys,omitempty"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	bucket := req.Bucket
	if bucket == "" {
		bucket = w.bucket
	}

	if req.MaxKeys == 0 {
		req.MaxKeys = 1000
	}

	objects := []map[string]interface{}{}
	
	for object := range w.client.ListObjects(ctx, bucket, minio.ListObjectsOptions{
		Prefix:    req.Prefix,
		Recursive: req.Recursive,
		MaxKeys:   req.MaxKeys,
	}) {
		if object.Err != nil {
			continue
		}
		
		objects = append(objects, map[string]interface{}{
			"key":          object.Key,
			"size":         object.Size,
			"etag":         object.ETag,
			"last_modified": object.LastModified,
			"content_type": object.ContentType,
		})
	}

	return json.Marshal(map[string]interface{}{
		"bucket":  bucket,
		"prefix":  req.Prefix,
		"objects": objects,
		"count":   len(objects),
	})
}

// Delete object
func (w *MinIOWorker) deleteObject(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		ObjectName string `json:"object_name"`
		Bucket     string `json:"bucket,omitempty"`
		VersionID  string `json:"version_id,omitempty"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	bucket := req.Bucket
	if bucket == "" {
		bucket = w.bucket
	}

	opts := minio.RemoveObjectOptions{}
	if req.VersionID != "" {
		opts.VersionID = req.VersionID
	}

	err := w.client.RemoveObject(ctx, bucket, req.ObjectName, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to delete: %w", err)
	}

	return json.Marshal(map[string]interface{}{
		"bucket":      bucket,
		"object_name": req.ObjectName,
		"deleted":     true,
	})
}

// Get presigned URL
func (w *MinIOWorker) getPresignedURL(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		ObjectName string        `json:"object_name"`
		Bucket     string        `json:"bucket,omitempty"`
		Expiry     time.Duration `json:"expiry_seconds,omitempty"`
		Method     string        `json:"method,omitempty"` // GET, PUT, DELETE
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	bucket := req.Bucket
	if bucket == "" {
		bucket = w.bucket
	}
	if req.Expiry == 0 {
		req.Expiry = 15 * time.Minute
	}
	if req.Method == "" {
		req.Method = "GET"
	}

	var presignedURL *url.URL
	var err error

	switch strings.ToUpper(req.Method) {
	case "GET":
		presignedURL, err = w.client.PresignedGetObject(ctx, bucket, req.ObjectName, req.Expiry, nil)
	case "PUT":
		presignedURL, err = w.client.PresignedPutObject(ctx, bucket, req.ObjectName, req.Expiry)
	default:
		return nil, fmt.Errorf("unsupported method: %s", req.Method)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to generate URL: %w", err)
	}

	return json.Marshal(map[string]interface{}{
		"bucket":      bucket,
		"object_name": req.ObjectName,
		"url":         presignedURL.String(),
		"method":      req.Method,
		"expiry":      req.Expiry.Seconds(),
	})
}

// Check if bucket exists
func (w *MinIOWorker) bucketExists(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		Bucket string `json:"bucket"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	bucket := req.Bucket
	if bucket == "" {
		bucket = w.bucket
	}

	exists, err := w.client.BucketExists(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("failed to check bucket: %w", err)
	}

	return json.Marshal(map[string]interface{}{
		"bucket":  bucket,
		"exists":  exists,
	})
}

// Create bucket
func (w *MinIOWorker) makeBucket(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		Bucket   string `json:"bucket"`
		Location string `json:"location,omitempty"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	if req.Bucket == "" {
		return nil, fmt.Errorf("bucket name required")
	}

	err := w.client.MakeBucket(ctx, req.Bucket, minio.MakeBucketOptions{
		Region: req.Location,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create bucket: %w", err)
	}

	return json.Marshal(map[string]interface{}{
		"bucket":  req.Bucket,
		"created": true,
	})
}

// List buckets
func (w *MinIOWorker) listBuckets(ctx context.Context, input json.RawMessage) ([]byte, error) {
	buckets, err := w.client.ListBuckets(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list buckets: %w", err)
	}

	bucketList := []map[string]interface{}{}
	for _, bucket := range buckets {
		bucketList = append(bucketList, map[string]interface{}{
			"name":         bucket.Name,
			"creation_date": bucket.CreationDate,
		})
	}

	return json.Marshal(map[string]interface{}{
		"buckets": bucketList,
		"count":   len(bucketList),
	})
}

// Get object metadata
func (w *MinIOWorker) getObjectInfo(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		ObjectName string `json:"object_name"`
		Bucket     string `json:"bucket,omitempty"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	bucket := req.Bucket
	if bucket == "" {
		bucket = w.bucket
	}

	stat, err := w.client.StatObject(ctx, bucket, req.ObjectName, minio.StatObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to stat object: %w", err)
	}

	return json.Marshal(map[string]interface{}{
		"bucket":        bucket,
		"object_name":   req.ObjectName,
		"size":          stat.Size,
		"etag":          stat.ETag,
		"last_modified": stat.LastModified,
		"content_type":  stat.ContentType,
		"metadata":      stat.UserMetadata,
	})
}

// Copy object
func (w *MinIOWorker) copyObject(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		SourceBucket      string `json:"source_bucket,omitempty"`
		SourceObject      string `json:"source_object"`
		DestinationBucket string `json:"dest_bucket,omitempty"`
		DestinationObject string `json:"dest_object"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	srcBucket := req.SourceBucket
	if srcBucket == "" {
		srcBucket = w.bucket
	}
	dstBucket := req.DestinationBucket
	if dstBucket == "" {
		dstBucket = w.bucket
	}

	srcOpts := minio.CopySrcOptions{
		Bucket: srcBucket,
		Object: req.SourceObject,
	}
	dstOpts := minio.CopyDestOptions{
		Bucket: dstBucket,
		Object: req.DestinationObject,
	}

	uploadInfo, err := w.client.CopyObject(ctx, dstOpts, srcOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to copy: %w", err)
	}

	return json.Marshal(map[string]interface{}{
		"source_bucket":      srcBucket,
		"source_object":      req.SourceObject,
		"destination_bucket": dstBucket,
		"destination_object": req.DestinationObject,
		"etag":               uploadInfo.ETag,
		"size":               uploadInfo.Size,
	})
}

// Move object (copy + delete)
func (w *MinIOWorker) moveObject(ctx context.Context, input json.RawMessage) ([]byte, error) {
	// Copy first
	_, err := w.copyObject(ctx, input)
	if err != nil {
		return nil, err
	}

	var req struct {
		SourceBucket string `json:"source_bucket,omitempty"`
		SourceObject string `json:"source_object"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	// Then delete source
	srcBucket := req.SourceBucket
	if srcBucket == "" {
		srcBucket = w.bucket
	}

	deleteReq, _ := json.Marshal(map[string]string{
		"bucket": srcBucket,
		"object_name": req.SourceObject,
	})

	_, err = w.deleteObject(ctx, deleteReq)
	if err != nil {
		return nil, fmt.Errorf("copied but failed to delete source: %w", err)
	}

	return json.Marshal(map[string]interface{}{
		"moved": true,
	})
}

// Sync local directory to MinIO
func (w *MinIOWorker) syncDirectory(ctx context.Context, input json.RawMessage) ([]byte, error) {
	var req struct {
		LocalPath   string            `json:"local_path"`
		Prefix      string            `json:"prefix,omitempty"`
		Bucket      string            `json:"bucket,omitempty"`
		Metadata    map[string]string `json:"metadata,omitempty"`
		Recursive   bool              `json:"recursive,omitempty"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	bucket := req.Bucket
	if bucket == "" {
		bucket = w.bucket
	}

	uploaded := []map[string]interface{}{}
	errors := []map[string]interface{}{}

	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			errors = append(errors, map[string]interface{}{
				"path":  path,
				"error": err.Error(),
			})
			return nil
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(req.LocalPath, path)
		if err != nil {
			return err
		}

		objectName := filepath.Join(req.Prefix, relPath)
		// Normalize for S3
		objectName = filepath.ToSlash(objectName)

		file, err := os.Open(path)
		if err != nil {
			errors = append(errors, map[string]interface{}{
				"path":  path,
				"error": err.Error(),
			})
			return nil
		}
		defer file.Close()

		contentType := mime.TypeByExtension(filepath.Ext(path))
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		uploadInfo, err := w.client.PutObject(ctx, bucket, objectName, file, info.Size(), minio.PutObjectOptions{
			ContentType: contentType,
			UserMetadata: req.Metadata,
		})
		if err != nil {
			errors = append(errors, map[string]interface{}{
				"path":   path,
				"object": objectName,
				"error":  err.Error(),
			})
			return nil
		}

		uploaded = append(uploaded, map[string]interface{}{
			"local_path":   path,
			"object_name":  objectName,
			"size":         uploadInfo.Size,
			"etag":         uploadInfo.ETag,
		})

		return nil
	}

	var walkErr error
	if req.Recursive {
		walkErr = filepath.Walk(req.LocalPath, walkFn)
	} else {
		// Just files in directory
		entries, err := os.ReadDir(req.LocalPath)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				path := filepath.Join(req.LocalPath, entry.Name())
				info, err := entry.Info()
				if err != nil {
					continue
				}
				walkFn(path, info, nil)
			}
		}
	}

	if walkErr != nil {
		return nil, walkErr
	}

	return json.Marshal(map[string]interface{}{
		"bucket":   bucket,
		"local_path": req.LocalPath,
		"prefix":   req.Prefix,
		"uploaded": len(uploaded),
		"errors":   len(errors),
		"files":    uploaded,
		"errors_detail": errors,
	})
}
