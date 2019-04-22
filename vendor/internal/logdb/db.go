package logdb

import (
	"fmt"
	"encoding/json"
	"github.com/boltdb/bolt"
	"time"
	"bytes"
	"encoding/gob"
	"github.com/prometheus/alertmanager/types"
	"crypto/md5"
    "encoding/hex"
    "os"
    "log"
    "github.com/luuphu25/alert2log_exporter/query"
    "github.com/luuphu25/alert2log_exporter/template"
    "context"
    "github.com/olivere/elastic"
    //"strings"
    //"github.com/prometheus/common/model"


)


type DBAlert struct {
	Alert *types.Alert 
	Status      string `json:"status"`
	Receivers   []string          `json:"receivers"`
	Fingerprint string              `json:"fingerprint"`
	TimeLog string `json:"timeLog"`
    IDstore string `json:"keyDB"`
    Metrics []string `json:"metrics_list`
}

type DBData struct {
    IDstore string `json:"keyDB"`
    Data map[string]*template.Query_struct
}



var prometheus_url = "http://61.28.251.119:9090"

func setupDB()(*bolt.DB, error) {
    db, err := bolt.Open("./DB/alert.db", 0644, &bolt.Options{Timeout: 1 * time.Second})
    if err != nil {
        return nil, fmt.Errorf("could not open db, %v", err)
    }
    // Bucket for alert
    err = db.Update(func(tx *bolt.Tx) error {
        _, err := tx.CreateBucketIfNotExists([]byte("alertBucket"))
        if err != nil {
        return fmt.Errorf("could not create root bucket: %v", err)
        }
       
        return nil
    })
    if err != nil {
        return nil, fmt.Errorf("could not set up buckets, %v", err)
    }
    // Bucket for pastData
    err = db.Update(func(tx *bolt.Tx) error {
        _, err := tx.CreateBucketIfNotExists([]byte("dataBucket"))
        if err != nil {
        return fmt.Errorf("could not create root bucket: %v", err)
        }
       
        return nil
    })
    if err != nil {
        return nil, fmt.Errorf("could not set up buckets, %v", err)
    }
    fmt.Printf("DB Setup Done\n")
    return db, nil
}

func (alert *DBAlert) save(db *bolt.DB) error {
	err := db.Update(func(tx *bolt.Tx) error {
        alertB, err := tx.CreateBucketIfNotExists([]byte("alertBucket"))
        if err != nil {
            return fmt.Errorf("create bucket: %s", err)
        }
        enc, err := alert.encode()
        if err != nil {
            return fmt.Errorf("could not encode Alert %s: %s", alert.IDstore, err)
        }
        err = alertB.Put([]byte(alert.IDstore), enc)
        return err
    })
    if err != nil {
    	return err
    }
    fmt.Printf("\n >> Store alert into DB success\n")
    return nil 
}

func (pastData *DBData) saveDB(db *bolt.DB) error {
	//fmt.Printf("Storing user with ID: ", user.ID)
	err := db.Update(func(tx *bolt.Tx) error {
        DataB, err := tx.CreateBucketIfNotExists([]byte("dataBucket"))
        if err != nil {
            return fmt.Errorf("create bucket: %s", err)
        }
        enc, err := pastData.encode()
        if err != nil {
            return fmt.Errorf("could not encode Alert %s: %s", pastData.IDstore, err)
        }
        err = DataB.Put([]byte(pastData.IDstore), enc)
        return err
    })
    if err != nil {
    	return err
    }
    fmt.Printf("\n >> Store pastdata into DB success\n")
    return nil 
}

func (alert *DBAlert) goEncode()([]byte, error){
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	err := enc.Encode(alert)
	 if err != nil {
        return nil, err
    }
    return buf.Bytes(), nil
}


func (pastData *DBData) goEncode()([]byte, error){
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	err := enc.Encode(pastData)
	 if err != nil {
        return nil, err
    }
    return buf.Bytes(), nil
}

func gobDecode(data []byte) (*DBAlert, error) {
    var a *DBAlert
    buf := bytes.NewBuffer(data)
    dec := gob.NewDecoder(buf)
    err := dec.Decode(&a)
    if err != nil {
        return nil, err
    }
    return a, nil
}

func (alert *DBAlert) encode() ([]byte, error) {
    enc, err := json.Marshal(alert)
    if err != nil {
        return nil, err
    }
    return enc, nil
}

func (pastData *DBData) encode() ([]byte, error) {
    enc, err := json.Marshal(pastData)
    if err != nil {
        return nil, err
    }
    return enc, nil
}

func decode(data []byte) (*DBAlert, error) {
    var alert *DBAlert
    err := json.Unmarshal(data, &alert)
    if err != nil {
        return nil, err
    }
    return alert, nil
}
/* 
func decode(data []byte) (*DBData, error) {
    var p *DBData
    err := json.Unmarshal(data, &p)
    if err != nil {
        return nil, err
    }
    return p, nil
}
 */

func GetUser(IDstore string, db *bolt.DB)(*DBAlert, error) {
    var p *DBAlert
    err := db.View(func(tx *bolt.Tx) error {
        var err error
        b := tx.Bucket([]byte("alertBucket"))
        k := []byte(IDstore)

        p, err = decode(b.Get(k))
        if err != nil {
            return err
        }
        return nil
    }) 
    if err != nil {
        fmt.Printf("Could not get Person ID %s \n\n", IDstore)
        return nil, err
    }
    return p, nil
}
func ListAlert(bucket string) ([]*DBAlert, error){
    db, _ := setupDB()
    defer db.Close()
   
    var listAlert = []*DBAlert{}
    err := db.View(func(tx *bolt.Tx) error {
        c := tx.Bucket([]byte(bucket)).Cursor()
        for k, v := c.First(); k != nil; k, v = c.Next() {
			//fmt.Printf("\n\n -- alert: ---\n")
            //fmt.Printf("Key=%s,\n Value= %s\n", k, v)
            p, err := decode(v)
            if err != nil {
                return err
            }
			listAlert = append(listAlert, p)
        }
        return nil
    })
    if err != nil {
        return nil, err
    }
    return listAlert, nil
}
func GetMD5Hash(text string) string {
    hasher := md5.New()
    hasher.Write([]byte(text))
    return hex.EncodeToString(hasher.Sum(nil))
}


func StoreDB(alert *DBAlert) (int, error) {
    var flag = -1
    db, err := setupDB()
    defer db.Close()
	// create hash to dedup alert
	HashValue := alert.Fingerprint + alert.Status + alert.Alert.StartsAt.String()
    alert.IDstore = GetMD5Hash(HashValue)
    fmt.Printf("\n ---- \n new alert with status: %s, ID: %s \n ---", alert.Status, alert.IDstore)
    
    _,err = GetUser(alert.IDstore,db)
	if err != nil {
        fmt.Printf("\n >> Saving data ....\n")
        alert.save(db)
        flag = 1
        if alert.Metrics != nil {
            savePastData(alert, db)
        }
        InsertEs(alert, "alert_filted")


        return flag, nil
	} else {
		fmt.Printf(" >>> Alert exists... skip")
        flag = 0
        return flag, err
    }
    return flag, nil
} 
func WriteAlert(alert DBAlert){

	fmt.Printf("Wrting alert")
	timestamp := int32(time.Now().Unix())
	times := fmt.Sprintf("%d", timestamp)
	date := time.Now().UTC().Format("01-02-2006")
	alert.TimeLog = times
	data, _ := json.MarshalIndent(alert, "", " ")
	var filename = "./Log_data/logAlert_" + date + ".json"
	_, err := os.Stat(filename)

	if err != nil {
		if os.IsNotExist(err){
			_, err := os.Create(filename)
			if err != nil {
				log.Fatal("Can't create log file", err)
			}
		}
	}
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal("Can't open new file", err)
	}
	

	defer f.Close()

	if _, err = f.Write(data); err != nil {
		log.Fatal("Can't write to file", err)
	}
	fmt.Printf("Write data to file success!\n")
	

}
func savePastData(alert *DBAlert, db *bolt.DB) error {
    var query_command string
    var past_data  template.Query_struct
    var StoreData DBData
    StoreData.IDstore = alert.IDstore
    start_time, end_time := query.CreateTime(alert.Alert.StartsAt)
    listData := make(map[string]*template.Query_struct, 10)
    
	for _,metric := range alert.Metrics {
        query_command = query.CreateQuery(prometheus_url, metric, start_time, end_time, "15s")
        past_data = query.Query_past(query_command)
        listData[metric] = &past_data

    }
    StoreData.Data = listData
   /*  db, err := setupDB()
    if err != nil {
        return err
    } */
    StoreData.saveDB(db)
    //defer db.Close()
    return nil
} 

func InsertEs(alert *DBAlert, indexName string){
    var url = "http://127.0.0.1:9200"
    client, err := elastic.NewClient(elastic.SetURL(url))

	if err != nil{
		panic(err)
	}

	ctx := context.Background()
	exists, err := client.IndexExists(indexName).Do(ctx)
	if err != nil {
		panic(err)
	}

	if !exists {
		_, err = client.CreateIndex(indexName).Do(ctx)
		if err != nil {
			panic(err)
		}
	}
	_, err = client.Index().Index(indexName).Type("doc").BodyJson(alert).Do(ctx)
	if err != nil {
		panic(err)
	}

	fmt.Printf("\nInsert to Elastic success\n")
}