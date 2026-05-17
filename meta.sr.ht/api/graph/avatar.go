package graph

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"image"
	"io"
	"path"

	"git.sr.ht/~sircmpwn/core-go/config"
	"git.sr.ht/~sircmpwn/core-go/errors"
	"git.sr.ht/~sircmpwn/core-go/objects"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"golang.org/x/image/draw"

	"git.sr.ht/~sircmpwn/meta.sr.ht/api/graph/model"

	_ "golang.org/x/image/webp"
	"image/jpeg"
	"image/png"
)

var InvalidImage errors.ErrorCode = "INVALID_IMAGE"

func avatarsEnabled(ctx context.Context) bool {
	conf := config.ForContext(ctx)
	_, ok := conf.Get("meta.sr.ht", "avatar-bucket")
	if !ok {
		return false
	}
	_, ok = conf.Get("meta.sr.ht", "avatar-prefix")
	if !ok {
		return false
	}
	_, err := objects.NewClient(conf)
	return err == nil
}

// Decodes an avatar and crops and scales it to no more than 512x512
func processAvatar(file io.ReadSeeker) (image.Image, string, hash.Hash, error) {
	sha := sha256.New()
	head, format, err := image.DecodeConfig(file)
	if err != nil {
		return nil, format, sha, err
	}

	if head.Width > 2048 || head.Height > 2048 {
		err := errors.New(InvalidImage,
			"Image exceeds maximum permitted dimensions (2048x2048)")
		return nil, format, sha, errors.Field(err, "avatar")
	}

	_, err = file.Seek(0, io.SeekStart)
	if err != nil {
		return nil, format, sha, err
	}

	rd := io.TeeReader(file, sha)

	src, _, err := image.Decode(rd)
	if err != nil {
		return nil, format, sha, err
	}

	w := src.Bounds().Max.X
	h := src.Bounds().Max.Y
	if w > h {
		w = h
	} else if h > w {
		h = w
	}
	srcRect := image.Rect(0, 0, w, h)
	if w > 512 || h > 512 {
		w = 512
		h = 512
	}

	dest := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.ApproxBiLinear.Scale(dest, dest.Rect, src, srcRect, draw.Over, nil)
	return dest, format, sha, nil
}

func uploadAvatar(
	ctx context.Context,
	user *model.User,
	file io.ReadSeeker,
	bucket, prefix string,
) (string, error) {
	conf := config.ForContext(ctx)
	client, err := objects.NewClient(conf)
	if err != nil {
		return "", err
	}

	image, format, sha, err := processAvatar(file)
	if err != nil {
		return "", err
	}

	sha.Write([]byte(user.Username))
	hash := sha.Sum(nil)
	name := hex.EncodeToString(hash[:])

	var (
		buf         bytes.Buffer
		contentType string
	)
	switch format {
	case "png":
		// Retain PNG as PNG, to preserve transparency and lossless
		// image quality
		name = fmt.Sprintf("%s.png", name)
		contentType = "image/png"
		err = png.Encode(&buf, image)
	default:
		// Convert anything else into JPEG
		name = fmt.Sprintf("%s.jpg", name)
		contentType = "image/jpeg"
		err = jpeg.Encode(&buf, image, &jpeg.Options{
			Quality: 95,
		})
	}

	if err != nil {
		return "", err
	}

	key := path.Join(prefix, name)
	opts := &s3.PutObjectInput{
		Bucket:        aws.String(bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(buf.Bytes()),
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(int64(buf.Len())),
		CacheControl:  aws.String("public, max-age=31536000, immutable"),
		ACL:           s3types.ObjectCannedACLPublicRead,
	}

	_, err = client.PutObject(ctx, opts)
	return name, err
}
