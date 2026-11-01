package main

import (
	"fmt"

	"github.com/SherClockHolmes/webpush-go"
)

func main() {
	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		panic(err)
	}
	fmt.Printf("PUSH_VAPID_PUBLIC=%s\nPUSH_VAPID_PRIVATE=%s\n", pub, priv)
}
