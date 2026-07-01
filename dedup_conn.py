import sqlite3, time

db = sqlite3.connect('/vol1/@appdata/wg-server/wg-server.db')

# Delete dupes: keep only 1 per user per connected minute
db.execute("DELETE FROM connection_log WHERE id NOT IN (SELECT MIN(id) FROM connection_log GROUP BY user_id, connected_at/60000)")
db.commit()

r = db.execute('SELECT COUNT(*) FROM connection_log').fetchone()
print('After dedup:', r[0])

rows = db.execute('SELECT id, username, connected_at, disconnected_at, rx_bytes, tx_bytes FROM connection_log ORDER BY connected_at').fetchall()
for row in rows:
    ctime = time.strftime('%H:%M:%S', time.localtime(row[2]/1000))
    dtime = 'online' if row[3]==0 else time.strftime('%H:%M:%S', time.localtime(row[3]/1000))
    print(f'  id={row[0]} {row[1]} {ctime} -> {dtime} rx={row[4]} tx={row[5]}')
