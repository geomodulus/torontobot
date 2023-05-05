// Package storage contains functions for saving data in places other than the DB.
package storage

import (
	"context"
	"fmt"
	"io"
	"log"
	"path"
	"time"

	"cloud.google.com/go/storage"
)

var (
	bucketName   = "dev.geomodul.us"
	objectPrefix = "dev-charts/"
)

func UploadToGCS(ctx context.Context, objectName string, src io.Reader) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second*50)
	defer cancel()

	client, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("storage.NewClient: %v", err)
	}
	defer client.Close()

	bucket := client.Bucket(bucketName)
	object := bucket.Object(path.Join(objectPrefix, objectName))
	writer := object.NewWriter(ctx)

	if _, err := io.Copy(writer, src); err != nil {
		return fmt.Errorf("io.Copy: %v", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("Writer.Close: %v", err)
	}

	log.Printf("Uploaded file to bucket: %s with object name: %s\nurl: https://%s\n",
		bucketName,
		objectName,
		bucketName+"/"+path.Join(objectPrefix, objectName))
	return nil
}
