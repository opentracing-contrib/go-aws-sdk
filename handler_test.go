package otaws

import (
	"errors"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/awstesting/mock"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/mocktracer"
	"strings"
	"testing"
)

func TestNonGlobalTracer(t *testing.T) {
	tracer := mocktracer.New()

	client := mock.NewMockClient(&aws.Config{
		Region: aws.String("us-west-2"),
	})

	AddOTHandlers(client, WithTracer(tracer))

	req := client.NewRequest(&request.Operation{
		Name:       "Test Operation",
		HTTPMethod: "POST",
	}, nil, nil)

	err := req.Send()
	if err != nil {
		t.Fatal("Expected request to succeed but failed:", err)
	}

	spans := tracer.FinishedSpans()

	if numSpans := len(spans); numSpans != 1 {
		t.Fatalf("Expected 1 span but found %d spans", numSpans)
	}

	span := spans[0]
	if span.OperationName != "Test Operation" {
		t.Errorf("Expected span to have operation name 'Test Operation' but was '%s'", span.OperationName)
	}
}

// Test requires running local instance of DynamoDB
func TestAWS(t *testing.T) {
	tracer := mocktracer.New()
	opentracing.InitGlobalTracer(tracer)

	client := mock.NewMockClient(&aws.Config{
		Region: aws.String("us-west-2"),
	})

	AddOTHandlers(client)

	req := client.NewRequest(&request.Operation{
		Name:       "Test Operation",
		HTTPMethod: "POST",
		HTTPPath:   "/foo/bar",
	}, nil, nil)

	err := req.Send()
	if err != nil {
		t.Fatal("Expected request to succeed but failed:", err)
	}

	spans := tracer.FinishedSpans()

	if numSpans := len(spans); numSpans != 1 {
		t.Fatalf("Expected 1 span but found %d spans", numSpans)
	}

	span := spans[0]

	if span.OperationName != "Test Operation" {
		t.Errorf("Expected span to have operation name 'Test Operation' but was '%s'", span.OperationName)
	}

	expectedTags := map[string]interface{}{
		"span.kind":        ext.SpanKindRPCClientEnum,
		"component":        "go-aws",
		"http.method":      "POST",
		"http.status_code": uint16(200),
		"peer.service":     "Mock",
	}

	for tag, expected := range expectedTags {
		if actual := span.Tag(tag); actual != expected {
			t.Errorf("Expected tag '%s' to have value '%v' but was '%v'", tag, expected, actual)
		}
	}

	url, ok := span.Tag("http.url").(string)
	if !ok {
		t.Errorf("Expected span to have tag 'http.url' of type string")
	}
	if !strings.HasSuffix(url, "/foo/bar") {
		t.Error("Expected tag 'http.url' to end with '/foo/bar' but was", url)
	}
}

func TestNilResponse(t *testing.T) {
	tracer := mocktracer.New()
	opentracing.InitGlobalTracer(tracer)

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("us-west-2"),
		Credentials: credentials.NewCredentials(credentials.ErrorProvider{
			Err:          errors.New("error credentials for test"),
			ProviderName: "test error provider",
		}),
	})
	if err != nil {
		t.Fatal("Failed to instantiate session:", err)
	}

	dbClient := dynamodb.New(sess)

	AddOTHandlers(dbClient.Client)

	_, err = dbClient.ListTables(&dynamodb.ListTablesInput{})
	if err == nil {
		t.Fatal("Expected error but request succeeded")
	}

	spans := tracer.FinishedSpans()
	if len(spans) != 1 {
		t.Fatalf("Expected 1 span but saw %d spans", len(spans))
	}

	errTag, ok := spans[0].Tag("error").(bool)
	if !ok {
		t.Fatal("Expected span to have an 'error' tag of type bool")
	} else if errTag != true {
		t.Fatal("Expected span's 'error' tag to be true but was false")
	}
}
