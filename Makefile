build:
	go build -o borgbackuptransactions_exporter .

test:
	go test ./...

clean:
	rm -f borgbackuptransactions_exporter
