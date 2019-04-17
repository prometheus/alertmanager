package Log

import (
	"fmt"
	"os"
	"github.com/prometheus/alertmanager/route"
	"github.com/prometheus/alertmanager/types"
)

type Alert struct {
	Labels       LabelSet  `json:"labels"`
	Annotations  LabelSet  `json:"annotations"`
	StartsAt     time.Time `json:"startsAt,omitempty"`
	EndsAt       time.Time `json:"endsAt,omitempty"`
	GeneratorURL string    `json:"generatorURL"`
}

// ExtendedAlert represents an alert as returned by the AlertManager's list alert API.
type ExtendedAlert struct {
	Alert types.Alerts
	//Status      types.AlertStatus `json:"status"`
	Status 		string
	Receivers   []string          `json:"receivers"`
	Fingerprint string            `json:"fingerprint"`
}


// LabelSet represents a collection of label names and values as a map.
type LabelSet map[LabelName]LabelValue

// LabelName represents the name of a label.
type LabelName string

// LabelValue represents the value of a label.
type LabelValue string

func AlertStore(ctx context.Context, alerts ...*types.Alert){
	var (
		//receiverFilter *regexp.Regexp
		// Initialize result slice to prevent api returning `null` when there
		// are no alerts present
		res      = []*Alert{}
		matchers = []*labels.Matcher{}

	)
	var tempAlert ExtendedAlert

	for a := range alerts.Next() {
		if err = alerts.Err(); err != nil {
			break
		}
		if err = ctx.Err(); err != nil {
			break
		}
		tempAlert.Alert = a

		routes := route.Match(a.Labels)
		receivers := make([]string, 0, len(routes))
		for _, r := range routes {
			receivers = append(receivers, r.RouteOpts.Receiver)
		}
		tempAlert.receivers = Receivers


		// Continue if the alert is resolved.
		/* if !a.Alert.EndsAt.IsZero() && a.Alert.EndsAt.Before(time.Now()) {
			
		} */
		if a.EndsAt.After(time.Now()) {
			tempAlert.Status = "firing"
		} else {
			tempAlert.Status = "resolved"
		}
		tempAlert.Fingerprint = a.Fingerprint().String()

		

		/* if !showActive && status.State == types.AlertStateActive {
			continue
		}

		if !showUnprocessed && status.State == types.AlertStateUnprocessed {
			continue
		}

		if !showSilenced && len(status.SilencedBy) != 0 {
			continue
		}

		if !showInhibited && len(status.InhibitedBy) != 0 {
			continue
		} */
/* 
		alert := &Alert{
			Alert:       &a.Alert,
			Status:      status,
			Receivers:   receivers,
			Fingerprint: a.Fingerprint().String(),
		

		res = append(res, alert)
		*/
		StoreResolved(tempAlert)
	}

}

func StoreResolved(alert *ExtendedAlert){

	//fmt.Printf("Wrting resolved alert")
	//timestamp := int32(time.Now().Unix())
	//times := []byte(fmt.Sprintf("%d", timestamp))
	date := time.Now().UTC().Format("01-02-2006")
	Alert := types.Alerts(alert)
	data, _ := json.MarshalIndent(Alert, "", " ")s
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

	if _, err = f.Write(times); err != nil {
		log.Fatal("Can't write timestamp to file", err)
	}
	if _, err = f.Write(data); err != nil {
		log.Fatal("Can't write to file", err)
	}
	fmt.Printf("Write data to file success!\n")
	

}

