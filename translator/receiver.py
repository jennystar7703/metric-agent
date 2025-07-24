import socket
import sys

# Configure the server
HOST = '0.0.0.0'
PORT = 8080
print(f"--- Simple Receiver listening on {HOST}:{PORT} ---")

# Create a socket
with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
    s.bind((HOST, PORT))
    s.listen()
    
    # Wait for one connection
    conn, addr = s.accept()
    with conn:
        print(f"--- ✅ Connection received from {addr} ---")
        data = b""
        # Loop to receive all data from the connection
        while True:
            chunk = conn.recv(1024)
            if not chunk:
                break
            data += chunk
        
        # --- This is the proof ---
        # Decode the raw bytes into text and print it.
        # This will show us the HTTP Headers and the JSON body.
        print("--- RAW DATA RECEIVED ---")
        print(data.decode('utf-8', errors='ignore'))
        print("-----------------------")
        
        # Send a simple HTTP 200 OK response back
        conn.sendall(b'HTTP/1.1 200 OK\r\n\r\nOK')