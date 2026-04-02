package main
import ("fmt";"log";"net/http";"os";"github.com/stockyard-dev/stockyard-saddlebag/internal/server";"github.com/stockyard-dev/stockyard-saddlebag/internal/store")
func main(){port:=os.Getenv("PORT");if port==""{port="8990"};dataDir:=os.Getenv("DATA_DIR");if dataDir==""{dataDir="./saddlebag-data"}
db,err:=store.Open(dataDir);if err!=nil{log.Fatalf("saddlebag: %v",err)};defer db.Close();srv:=server.New(db)
fmt.Printf("\n  Saddlebag — file manager\n  Dashboard:  http://localhost:%s/ui\n  API:        http://localhost:%s/api\n\n",port,port)
log.Printf("saddlebag: listening on :%s",port);log.Fatal(http.ListenAndServe(":"+port,srv))}
