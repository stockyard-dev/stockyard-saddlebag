package store
import ("database/sql";"fmt";"os";"path/filepath";"time";_ "modernc.org/sqlite")
type DB struct{db *sql.DB}
type Backup struct {
	ID string `json:"id"`
	Name string `json:"name"`
	Source string `json:"source"`
	Destination string `json:"destination"`
	SizeBytes int `json:"size_bytes"`
	Status string `json:"status"`
	Schedule string `json:"schedule"`
	LastRunAt string `json:"last_run_at"`
	CreatedAt string `json:"created_at"`
}
func Open(d string)(*DB,error){if err:=os.MkdirAll(d,0755);err!=nil{return nil,err};db,err:=sql.Open("sqlite",filepath.Join(d,"saddlebag.db")+"?_journal_mode=WAL&_busy_timeout=5000");if err!=nil{return nil,err}
db.Exec(`CREATE TABLE IF NOT EXISTS backups(id TEXT PRIMARY KEY,name TEXT NOT NULL,source TEXT DEFAULT '',destination TEXT DEFAULT '',size_bytes INTEGER DEFAULT 0,status TEXT DEFAULT 'completed',schedule TEXT DEFAULT '',last_run_at TEXT DEFAULT '',created_at TEXT DEFAULT(datetime('now')))`)
return &DB{db:db},nil}
func(d *DB)Close()error{return d.db.Close()}
func genID()string{return fmt.Sprintf("%d",time.Now().UnixNano())}
func now()string{return time.Now().UTC().Format(time.RFC3339)}
func(d *DB)Create(e *Backup)error{e.ID=genID();e.CreatedAt=now();_,err:=d.db.Exec(`INSERT INTO backups(id,name,source,destination,size_bytes,status,schedule,last_run_at,created_at)VALUES(?,?,?,?,?,?,?,?,?)`,e.ID,e.Name,e.Source,e.Destination,e.SizeBytes,e.Status,e.Schedule,e.LastRunAt,e.CreatedAt);return err}
func(d *DB)Get(id string)*Backup{var e Backup;if d.db.QueryRow(`SELECT id,name,source,destination,size_bytes,status,schedule,last_run_at,created_at FROM backups WHERE id=?`,id).Scan(&e.ID,&e.Name,&e.Source,&e.Destination,&e.SizeBytes,&e.Status,&e.Schedule,&e.LastRunAt,&e.CreatedAt)!=nil{return nil};return &e}
func(d *DB)List()[]Backup{rows,_:=d.db.Query(`SELECT id,name,source,destination,size_bytes,status,schedule,last_run_at,created_at FROM backups ORDER BY created_at DESC`);if rows==nil{return nil};defer rows.Close();var o []Backup;for rows.Next(){var e Backup;rows.Scan(&e.ID,&e.Name,&e.Source,&e.Destination,&e.SizeBytes,&e.Status,&e.Schedule,&e.LastRunAt,&e.CreatedAt);o=append(o,e)};return o}
func(d *DB)Update(e *Backup)error{_,err:=d.db.Exec(`UPDATE backups SET name=?,source=?,destination=?,size_bytes=?,status=?,schedule=?,last_run_at=? WHERE id=?`,e.Name,e.Source,e.Destination,e.SizeBytes,e.Status,e.Schedule,e.LastRunAt,e.ID);return err}
func(d *DB)Delete(id string)error{_,err:=d.db.Exec(`DELETE FROM backups WHERE id=?`,id);return err}
func(d *DB)Count()int{var n int;d.db.QueryRow(`SELECT COUNT(*) FROM backups`).Scan(&n);return n}

func(d *DB)Search(q string, filters map[string]string)[]Backup{
    where:="1=1"
    args:=[]any{}
    if q!=""{
        where+=" AND (name LIKE ?)"
        args=append(args,"%"+q+"%");
    }
    if v,ok:=filters["source"];ok&&v!=""{where+=" AND source=?";args=append(args,v)}
    if v,ok:=filters["status"];ok&&v!=""{where+=" AND status=?";args=append(args,v)}
    rows,_:=d.db.Query(`SELECT id,name,source,destination,size_bytes,status,schedule,last_run_at,created_at FROM backups WHERE `+where+` ORDER BY created_at DESC`,args...)
    if rows==nil{return nil};defer rows.Close()
    var o []Backup;for rows.Next(){var e Backup;rows.Scan(&e.ID,&e.Name,&e.Source,&e.Destination,&e.SizeBytes,&e.Status,&e.Schedule,&e.LastRunAt,&e.CreatedAt);o=append(o,e)};return o
}

func(d *DB)Stats()map[string]any{
    m:=map[string]any{"total":d.Count()}
    rows,_:=d.db.Query(`SELECT status,COUNT(*) FROM backups GROUP BY status`)
    if rows!=nil{defer rows.Close();by:=map[string]int{};for rows.Next(){var s string;var c int;rows.Scan(&s,&c);by[s]=c};m["by_status"]=by}
    return m
}
