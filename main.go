package main

import (
   "context"
   "flag"
   "log"

   "github.com/dorzeidman/conductor-terraform-provider/internal/provider"

   "github.com/hashicorp/terraform-plugin-framework/providerserver"
)

func main() {
   var debug bool

   flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers like delve")
   flag.Parse()

   opts := providerserver.ServeOpts{
       Address: "hashicorp.com/dorzeidman/conductor",
       Debug:   debug,
   }

   err := providerserver.Serve(context.Background(), provider.New(), opts)

   if err != nil {
       log.Fatal(err.Error())
   }
}