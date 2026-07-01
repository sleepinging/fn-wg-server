import sqlite3
db = sqlite3.connect('/vol1/@appdata/wg-server/wg-server.db')

db.execute("UPDATE users SET created_at = CAST(strftime('%s', created_at) AS INTEGER) * 1000 WHERE typeof(created_at) = 'text'")
db.execute("UPDATE users SET updated_at = CAST(strftime('%s', updated_at) AS INTEGER) * 1000 WHERE typeof(updated_at) = 'text'")
db.execute("UPDATE system_log SET created_at = CAST(strftime('%s', created_at) AS INTEGER) * 1000 WHERE typeof(created_at) = 'text'")
db.commit()

r = db.execute("SELECT typeof(created_at) FROM users LIMIT 1").fetchone()
print('created_at type:', r[0])
r2 = db.execute("SELECT id, username, created_at FROM users LIMIT 3").fetchall()
for row in r2:
    print(row)
db.close()
