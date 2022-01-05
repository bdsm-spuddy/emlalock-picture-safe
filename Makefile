TARGET=picture_lock

SRC:=$(shell echo *.go)

.DUMMY: ALL

ALL: $(TARGET) $(TARGET).exe $(TARGET).darwin

$(TARGET): $(SRC)
	go build -trimpath -o $@ $(SRC)

$(TARGET).exe : $(SRC)
	GOOS=windows GOARCH=amd64 go build -trimpath -o $@ $(SRC)

$(TARGET).darwin : $(SRC)
	GOOS=darwin GOARCH=amd64 go build -trimpath -o $@ $(SRC)

clean:
	/bin/rm -f $(TARGET)
