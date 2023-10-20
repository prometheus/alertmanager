package mongodb

import (
	"context"
	"fmt"
	"github.com/prometheus/alertmanager/types"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	mongodb "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Notifier struct {
	conf          *config.MongoDbConfig
	tmpl          *template.Template
	mongodbClient *mongodb.Client
	logger        log.Logger
	retrier       *notify.Retrier
}

type Document struct {
	Time      primitive.DateTime `json:"time" bson:"time"`
	AlertData *template.Data
}

func (n Notifier) Notify(ctx context.Context, alert ...*types.Alert) (bool, error) {
	collection := n.mongodbClient.Database(n.conf.Database).Collection(n.conf.Collection)
	data := notify.GetTemplateData(ctx, n.tmpl, alert, n.logger)

	doc := Document{
		Time:      primitive.NewDateTimeFromTime(time.Now()),
		AlertData: data,
	}

	res, err := collection.InsertOne(context.Background(), doc)
	if err != nil {
		//log.Fatal(err)
	}
	fmt.Println("Inserted document with time:", res.InsertedID)

	return true, nil
}

// New returns a new notifier that uses the mongodb.
func New(c *config.MongoDbConfig, t *template.Template, l log.Logger) (*Notifier, error) {
	//connectionString := fmt.Sprintf("mongodb://%s:%s@%s:%s", c.Username, c.Password, c.Url, c.Port)
	connectionString := fmt.Sprintf("mongodb://%s:%s", c.Url, c.Port)

	clientOptions := options.Client().ApplyURI(connectionString)
	// Connect to MongoDB
	client, err := mongodb.Connect(context.Background(), clientOptions)
	if err != nil {
		return nil, err
	}

	n := &Notifier{
		conf:          c,
		tmpl:          t,
		mongodbClient: client,
		logger:        l,
		retrier:       &notify.Retrier{},
	}

	return n, nil
}
