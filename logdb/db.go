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
)


type DBAlert struct {
	Alert *types.Alert 
	Status      string `json:"status"`
	Receivers   []string          `json:"receivers"`
	Fingerprint string            `json:"fingerprint"`
	TimeLog string `json:"timeLog"`
	IDstore string `json:"keyDB"`
}



func setupDB()(*bolt.DB, error) {
    db, err := bolt.Open("alert.db", 0644, &bolt.Options{Timeout: 1 * time.Second})
    if err != nil {
        return nil, fmt.Errorf("could not open db, %v", err)
    }
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
    fmt.Printf("DB Setup Done\n")
    return db, nil
}

func (alert *DBAlert) save(db *bolt.DB) error {
	//fmt.Printf("Storing user with ID: ", user.ID)
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
    fmt.Printf("\nStore data into DB success\n")
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

func decode(data []byte) (*DBAlert, error) {
    var alert *DBAlert
    err := json.Unmarshal(data, &alert)
    if err != nil {
        return nil, err
    }
    return alert, nil
}

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
func GetMD5Hash(text string) string {
    hasher := md5.New()
    hasher.Write([]byte(text))
    return hex.EncodeToString(hasher.Sum(nil))
}


func StoreDB(alert *DBAlert) (int, error) {
    var flag = -1
    db, err := setupDB()
    defer db.Close()
	//user1 := &User{"1234", "Nam", 23, "active", "11111"}
	HashValue := alert.Fingerprint + alert.Status + alert.Alert.StartsAt.String()
    alert.IDstore = GetMD5Hash(HashValue)
    fmt.Printf("\n ---- \n new alert with status: %s, ID: %s \n ---", alert.Status, alert.IDstore)
	//fmt.Printf(user1.Name +"\n\n")
	_,err = GetUser(alert.IDstore,db)
	if err != nil {
        fmt.Printf("Saving data \n\n\n")
        alert.save(db)
        flag = 1
        return flag, nil
	} else {
		fmt.Printf("Data exists")
        //fmt.Printf(user2.Name)
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

	/// if _, err = f.Write(times); err != nil {
	//	log.Fatal("Can't write timestamp to file", err)
	//} 
	if _, err = f.Write(data); err != nil {
		log.Fatal("Can't write to file", err)
	}
	fmt.Printf("Write data to file success!\n")
	

}