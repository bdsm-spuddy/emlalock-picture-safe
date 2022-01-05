# picture_lock

In [version 1](https://bdsm.spuddy.org/writings/Safe/) I wrote a GUI that would
generate image files suitable for [Emlalock](https://www.emlalock.com/).
This worked fine because the software ran on your PC and just communicated
via serial port to the safe

[Version 2](https://bdsm.spuddy.org/writings/Safe_v2/) of the safe uses an
ESP8266 to contol and provide the UI.  This is more limited in scope, but
provides a kind-of-API that can be called.

So this program adds the "missing" functionality.

## What it does

Emlalock lets you upload a picture of your combination lock, then delete
the picture.  Once the lock is over you can get a copy of the picture back
and so undo the lock.

So what this software does is generate a _random_ password, locks the safe
with the password then generates an image with the password embedded.  It
puts the password into JPEG _metadata_ so the image itself doesn't show
anything.  You can then upload this to Emlalock and delete it.

At the end of the session you can download the picture and then use this
software to parse the image and unlock the safe.

You never see the combination (well, password!) and don't have to worry about
out-of-focus images.

## Configuration.

The software needs to know three things:

* The safe network address (name or IP address)
* (Optional) The username to access the safe
* (Optional) The password to access the safe

Those optional values are only needed if the safe has been setup.

These values can be passed on the command line, or stored in a configuration
file.  The configuration file is called `.picture_lock` and is stored in your
home directory (so something like `/home/bdsm/.picture_lock` on Linux, or
`/Users/bdsm/.picture_lock` on MacOS, or `C:\Documents and Settings\bdsm\.picture_lock` or `C:\Users\bdsm\.picture_lock` on Windows).

The format of the file is a simple JSON file:

```
{
	"Safe": "safe.local",
	"User": "username",
	"Pass": "password"
}
```

If you don't wish to use the configuration (or if you wish to override those
values) then you can use the command line options:

e.g.

```
-safe safe.local -user username -pass password
```

## Examples

In the following examples we will assume the configuration file is present.

### Create a lock

```
picture_lock -lock -source original_image.jpg lock_image.jpg
```

This will lock the safe, then take the file `original_image.jpg` and
create a new file `lock_image.jpg` with the password embedded into it.
This will be the file to upload to Emlalock.

A file `lock_template.jpg` has been provided to use as a sample, but another
JPEG could be used (a picture of your cat?).

### Test a lock

```
picture_lock -test lock_image.jpg
```

This will take the lock image and verify the password embedded into it
will work with the safe.   This is a useful step before closing the safe door.

### Unlock the safe

```
picture_lock -unlock lock_image.jpg
```

This will take the lock image and use the password embedded into it to try
and unlock the safe.

### Check the safe status

```
picture_lock -status
```

This should simply return if the safe is locked or not.

## Example behaviour.

1. Open the safe door from the safe Web UI, and keep it open and unlocked.

2. Generate a new lock

```
% ./picture_lock -lock -source lock_template.jpg testlock.jpg
Creating a new lock
testlock.jpg created.
```

3. Verify the image works:

```
% ./picture_lock -test testlock.jpg
Passwords match
```

4. Put your key in the safe, close the door and lock it.

5. Start a session in Emalock, using the new file `testlock.jpg` as your image.

6. Delete `testlock.jpg`

7. At the end of the session, retrieve the image.  It may be called something
odd, like `5b2xrqrt40.jpg`

8. Unlock the safe

```
% ./picture_lock -unlock 5b2xrqrt40.jpg
Safe unlocked
```

9. From the Safe Web UI open the safe and retrieve the key.
