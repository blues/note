// The Notecard supports wildcard domain names, the extreme case being that it will accept a connection
// to any server that has "*" as the subject in its certificate.
openssl req -new -newkey rsa:2048 -x509 -sha256 -days 10000 -nodes -subj "/CN=*" -out minhub.crt -keyout minhub.key
openssl x509 -inform PEM -in minhub.crt -text

