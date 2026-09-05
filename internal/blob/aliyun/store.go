package aliyun

import (
	"context"
	"errors"
	"io"
	"path"
	"strings"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss/credentials"
)

type Config struct {
	Endpoint, Region, Bucket, AccessKeyID, AccessKeySecret, PathPrefix string
	UsePathStyle                                                       bool
}
type Store struct {
	client         *oss.Client
	bucket, prefix string
}

func New(cfg Config) (*Store, error) {
	if strings.TrimSpace(cfg.Region) == "" || strings.TrimSpace(cfg.Bucket) == "" || strings.TrimSpace(cfg.AccessKeyID) == "" || strings.TrimSpace(cfg.AccessKeySecret) == "" {
		return nil, errors.New("incomplete Aliyun OSS configuration")
	}
	ossCfg := oss.LoadDefaultConfig().WithRegion(cfg.Region).WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.AccessKeySecret))
	if strings.TrimSpace(cfg.Endpoint) != "" {
		ossCfg = ossCfg.WithEndpoint(strings.TrimSpace(cfg.Endpoint))
	}
	if cfg.UsePathStyle {
		ossCfg = ossCfg.WithUsePathStyle(true)
	}
	return &Store{client: oss.NewClient(ossCfg), bucket: strings.TrimSpace(cfg.Bucket), prefix: strings.Trim(strings.TrimSpace(cfg.PathPrefix), "/")}, nil
}

func (s *Store) key(key string) (string, error) {
	if key == "" || strings.Contains(key, "\\") || strings.HasPrefix(key, "/") || path.Clean(key) != key || strings.HasPrefix(key, "../") {
		return "", errors.New("invalid object key")
	}
	if s.prefix == "" {
		return key, nil
	}
	return s.prefix + "/" + key, nil
}

func (s *Store) Put(ctx context.Context, key string, body io.Reader, metadata application.BlobMetadata) error {
	key, err := s.key(key)
	if err != nil {
		return err
	}
	_, err = s.client.PutObject(ctx, &oss.PutObjectRequest{Bucket: oss.Ptr(s.bucket), Key: oss.Ptr(key), Body: body, ContentType: oss.Ptr(metadata.ContentType)})
	return err
}
func (s *Store) Open(ctx context.Context, key string) (io.ReadCloser, application.BlobInfo, error) {
	key, err := s.key(key)
	if err != nil {
		return nil, application.BlobInfo{}, err
	}
	result, err := s.client.GetObject(ctx, &oss.GetObjectRequest{Bucket: oss.Ptr(s.bucket), Key: oss.Ptr(key)})
	if err != nil {
		return nil, application.BlobInfo{}, err
	}
	return result.Body, application.BlobInfo{Size: result.ContentLength}, nil
}
func (s *Store) Stat(ctx context.Context, key string) (application.BlobInfo, error) {
	key, err := s.key(key)
	if err != nil {
		return application.BlobInfo{}, err
	}
	result, err := s.client.HeadObject(ctx, &oss.HeadObjectRequest{Bucket: oss.Ptr(s.bucket), Key: oss.Ptr(key)})
	if err != nil {
		return application.BlobInfo{}, err
	}
	return application.BlobInfo{Size: result.ContentLength}, nil
}
func (s *Store) Delete(ctx context.Context, key string) error {
	key, err := s.key(key)
	if err != nil {
		return err
	}
	_, err = s.client.DeleteObject(ctx, &oss.DeleteObjectRequest{Bucket: oss.Ptr(s.bucket), Key: oss.Ptr(key)})
	return err
}
