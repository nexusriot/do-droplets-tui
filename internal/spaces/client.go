package spaces

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type Client struct {
	s3     *s3.Client
	region string
}

func New(accessKey, secretKey, region string) (*Client, error) {
	region = strings.TrimSpace(region)
	if region == "" {
		region = "fra1"
	}
	if strings.TrimSpace(accessKey) == "" {
		return nil, fmt.Errorf("spaces access key is empty")
	}

	endpoint := fmt.Sprintf("https://%s.digitaloceanspaces.com", region)

	cfg := aws.Config{
		Region: region,
		Credentials: credentials.NewStaticCredentialsProvider(
			strings.TrimSpace(accessKey),
			strings.TrimSpace(secretKey),
			"",
		),
		EndpointResolverWithOptions: aws.EndpointResolverWithOptionsFunc(
			func(service, reg string, options ...interface{}) (aws.Endpoint, error) {
				return aws.Endpoint{URL: endpoint, SigningRegion: region}, nil
			},
		),
	}

	cl := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})

	return &Client{s3: cl, region: region}, nil
}

type BucketRow struct {
	Name    string
	Created time.Time
}

type ObjectRow struct {
	Key          string
	Size         int64
	LastModified time.Time
	StorageClass string
}

func (c *Client) ListBuckets(ctx context.Context) ([]BucketRow, error) {
	out, err := c.s3.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil, err
	}
	rows := make([]BucketRow, 0, len(out.Buckets))
	for _, b := range out.Buckets {
		row := BucketRow{Name: aws.ToString(b.Name)}
		if b.CreationDate != nil {
			row.Created = *b.CreationDate
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (c *Client) CreateBucket(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("bucket name is required")
	}
	_, err := c.s3.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(name),
		CreateBucketConfiguration: &types.CreateBucketConfiguration{
			LocationConstraint: types.BucketLocationConstraint(c.region),
		},
	})
	return err
}

func (c *Client) DeleteBucket(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("bucket name is required")
	}
	_, err := c.s3.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(name)})
	return err
}

func (c *Client) ListObjects(ctx context.Context, bucket, prefix string) ([]ObjectRow, error) {
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return nil, fmt.Errorf("bucket name is required")
	}

	var out []ObjectRow
	paginator := s3.NewListObjectsV2Paginator(c.s3, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, obj := range page.Contents {
			row := ObjectRow{
				Key:  aws.ToString(obj.Key),
				Size: aws.ToInt64(obj.Size),
			}
			if obj.LastModified != nil {
				row.LastModified = *obj.LastModified
			}
			row.StorageClass = string(obj.StorageClass)
			out = append(out, row)
		}
	}
	return out, nil
}

func (c *Client) DeleteObject(ctx context.Context, bucket, key string) error {
	bucket = strings.TrimSpace(bucket)
	key = strings.TrimSpace(key)
	if bucket == "" || key == "" {
		return fmt.Errorf("bucket and key are required")
	}
	_, err := c.s3.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	return err
}

func (c *Client) DeleteObjects(ctx context.Context, bucket string, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	objs := make([]types.ObjectIdentifier, 0, len(keys))
	for _, k := range keys {
		objs = append(objs, types.ObjectIdentifier{Key: aws.String(k)})
	}
	_, err := c.s3.DeleteObjects(ctx, &s3.DeleteObjectsInput{
		Bucket: aws.String(bucket),
		Delete: &types.Delete{Objects: objs},
	})
	return err
}
