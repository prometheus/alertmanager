package main 

import (
	"fmt"
	"encoding/json"
	"github.com/boltdb/bolt"
	//"log"
	"time"
	"bytes"
    //"runtime"
	//"encoding/json"
	"github.com/prometheus/alertmanager/types"

    "encoding/gob"
)
type User struct {
	Alert *types.Alert 
	Status      string `json:"status"`
	Receivers   []string          `json:"receivers"`
	Fingerprint string            `json:"fingerprint"`
	TimeLog string `json:"timeLog"`
	IDstore string `json:"keyDB"`
}



func setupDB()(*bolt.DB, error) {
    db, err := bolt.Open("alert.db", 0600, &bolt.Options{Timeout: 1 * time.Second})
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
    fmt.Println("DB Setup Done")
    return db, nil
}

func (user *User) save(db *bolt.DB) error {
	fmt.Printf("Storing user with ID: ", user.IDstore)
	err := db.Update(func(tx *bolt.Tx) error {
        people, err := tx.CreateBucketIfNotExists([]byte("alertBucket"))
        if err != nil {
            return fmt.Errorf("create bucket: %s", err)
        }
        enc, err := user.encode()
        if err != nil {
            return fmt.Errorf("could not encode Person %s: %s", user.IDstore, err)
        }
        err = people.Put([]byte(user.IDstore), enc)
        return err
    })
    if err != nil {
    	return err
    }
    fmt.Printf("Store data success")
    return nil 
}

func (user *User) goEncode()([]byte, error){
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	err := enc.Encode(user)
	 if err != nil {
        return nil, err
    }
    return buf.Bytes(), nil
}

func gobDecode(data []byte) (*User, error) {
    var p *User
    buf := bytes.NewBuffer(data)
    dec := gob.NewDecoder(buf)
    err := dec.Decode(&p)
    if err != nil {
        return nil, err
    }
    return p, nil
}

func (p *User) encode() ([]byte, error) {
    enc, err := json.Marshal(p)
    if err != nil {
        return nil, err
    }
    return enc, nil
}

func decode(data []byte) (*User, error) {
    var p *User
    err := json.Unmarshal(data, &p)
    if err != nil {
        return nil, err
    }
    return p, nil
}

func GetUser(IDstore string, db *bolt.DB)(*User, error) {
    var p *User
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
func List(bucket string, db *bolt.DB) {
    db.View(func(tx *bolt.Tx) error {
        c := tx.Bucket([]byte(bucket)).Cursor()
        for k, v := c.First(); k != nil; k, v = c.Next() {
			fmt.Printf("\n\n -- alert: ---\n")
			fmt.Printf("Key=%s,\n Value= %s\n", k, v)
			
        }
        return nil
    })
}

func main(){
	db, _ := setupDB()
	/* user1 := &User{"1234", "Nam", 23, "active", "11111"}
	var user2 *User
	fmt.Printf(user1.Name +"\n\n")
	user2 ,err = GetUser(user1.ID, db)
	if err != nil {
		user1.save(db)
	} else {
		fmt.Printf("Data exists")
		fmt.Printf(user2.Name)
	} */
	List("alertBucket", db)
}