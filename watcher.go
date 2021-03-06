package main

import (
	"fmt"
	"io"
	"log"

	"cloud.google.com/go/firestore"
	"github.com/lightningnetwork/lnd/lnrpc"
	"golang.org/x/net/context"
)

type Message struct {
	Invoice string `json:"invoice,omitempty"`
	Settled bool   `json:"settled,omitempty"`
}

// func watchPayments() {
// 	//TODO: A better way is to watch for payments and then
// 	// update firebase.
// 	ticker := time.NewTicker(15 * time.Second)
// 	go func() {
// 		for {
// 			select {
// 			case <-ticker.C:
// 				checkPayments()
// 			}
// 		}
// 	}()
// }

func checkPayments() {
	c, clean := getClient()
	defer clean()

	// 1st get unsettled message payment hashes
	it := firebaseDb.Collection("messages").Where("settled", "==", false).Documents(context.Background())
	snapshot, err := it.GetAll()
	if err != nil {
		log.Fatalln("Failed to get documents ", err)
		return
	}
	for _, s := range snapshot {
		invoice := s.Data()["invoice"].(string)
		decoded, err := c.DecodePayReq(context.Background(), &lnrpc.PayReqString{PayReq: invoice})
		if err != nil {
			fmt.Println("Failed to decode payreq")
			continue
		}

		lnInvoice, err := c.LookupInvoice(context.Background(), &lnrpc.PaymentHash{RHashStr: decoded.GetPaymentHash()})
		if err != nil {
			// It's possible that invoice generated with a test lnd won't appear in prod lnd.
			// Best approach is to separate them in the DB, but for now, just ignore them.
			fmt.Println("Failed to find invoice ", err)
		} else {
			if lnInvoice.GetSettled() {
				_, err := s.Ref.Update(context.Background(), []firestore.Update{{Path: "settled", Value: true}})
				if err != nil {
					log.Println("Update failed ", err)
				} else {
					log.Println("Updated ", invoice)
				}
			}
		}

	}
}

func watchInvoices() {
	c, clean := getClient()
	defer clean()

	sub, err := c.SubscribeInvoices(context.Background(), &lnrpc.InvoiceSubscription{})
	if err != nil {
		fmt.Println(err)
		return
	}
	for {
		invoice, err := sub.Recv()
		if err == io.EOF {
			sub.CloseSend()
		}
		if err != nil {
			fmt.Println(err)
			return
		}

		if invoice.GetSettled() {
			fmt.Println("Received ", invoice.GetPaymentRequest())
			it := firebaseDb.Collection("messages").Where("invoice", "==", invoice.GetPaymentRequest()).Limit(1).Documents(context.Background())
			snapshot, err := it.GetAll()
			if err != nil {
				fmt.Println("Couldn't find invoice in firebase")
				continue
			}
			for _, s := range snapshot {
				s.Ref.Update(context.Background(), []firestore.Update{{Path: "settled", Value: true}})
			}
		}
	}
}
