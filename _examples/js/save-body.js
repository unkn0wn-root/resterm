client.test("image is saved", function () {
  if (!response.isBinary) {
    throw new Error("body should be binary");
  }
  response.saveBody("resterm-example.png");
});
