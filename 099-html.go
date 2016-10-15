package main

var header = `<!DOCTYPE html>
<html>
<head>
	<meta http-equiv="Content-Type" content="text/html; charset=utf-8">
	<meta name="viewport" content="width=device-width">
	<meta name="theme-color" content="#375EAB">
	<title>` + version + `</title>
` +
	`<style>
body{
  color: green;
  background-color:   #E0EBF5;
}
.box {
border: 1px solid black;
min-width: 100px;
margin: 10px;
float: left;
clear: none;
padding: 10px;
}
</style>
<body>

`

var footer = `</body></html>`

var form = `
<h1>Thumber</h1>
<h2>Thumbnail Server</h2>
<h3> Upload a file </h3>
<form id="post" action="/upload" enctype="multipart/form-data" method="POST">
		<input name="file" type="file" required/></input>
    <br><input id="upload-submit" type="submit" value="upload" />
</form>
<pre style="background-color: lightgrey; width: 300px;">
Public API:

Original Size: /fileID
Resize: /width/height/fileID
Resize: /fileID/width/height (alt)
Upload: POST /upload

Example: /640/480/cat.jpeg

</pre>
`
