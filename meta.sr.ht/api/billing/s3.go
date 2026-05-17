package billing

import (
	"bytes"
	"context"
	"io"
	"path"

	"git.sr.ht/~sircmpwn/core-go/config"
	"git.sr.ht/~sircmpwn/core-go/objects"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"git.sr.ht/~sircmpwn/meta.sr.ht/api/graph/model"
)

// Uploads an invoice to S3.
func UploadInvoice(
	ctx context.Context,
	invoice *model.Invoice,
) error {
	conf := config.ForContext(ctx)

	sc, err := objects.NewClient(conf)
	if err != nil {
		return err
	}

	bucket, _ := conf.Get("meta.sr.ht::billing", "invoice-bucket")
	prefix, _ := conf.Get("meta.sr.ht::billing", "invoice-prefix")

	s3key := path.Join(prefix, invoice.UUID.String()+".pdf")

	var buffer bytes.Buffer
	err = GenerateInvoice(ctx, &buffer, invoice)
	if err != nil {
		return err
	}

	reader := bytes.NewReader(buffer.Bytes())

	_, err = sc.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(s3key),
		Body:        reader,
		ContentType: aws.String("application/pdf"),
	})
	return err
}

func DownloadInvoice(
	ctx context.Context,
	invoice *model.Invoice,
) (io.ReadCloser, error) {
	conf := config.ForContext(ctx)

	sc, err := objects.NewClient(conf)
	if err != nil {
		return nil, err
	}

	bucket, _ := conf.Get("meta.sr.ht::billing", "invoice-bucket")
	prefix, _ := conf.Get("meta.sr.ht::billing", "invoice-prefix")

	s3key := path.Join(prefix, invoice.UUID.String()+".pdf")
	object, err := sc.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(s3key),
	})
	if err != nil {
		return nil, err
	}

	return object.Body, nil
}
