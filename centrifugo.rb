class Centrifugo < Formula
  desc "Centrifugo"
  homepage "https://github.com/centrifugal/centrifugo"
  url "https://github.com/centrifugal/centrifugo/releases/download/v1.8.0/centrifugo-1.8.0-darwin-amd64.zip"
  sha256 "1582c07f66f91b401121fece225662b6b4f7836fdc9043ad5ec959a729b06253"

  def install
    system "chmod", "+x", "centrifugo"
    system "mkdir", "#{prefix}/bin"
    system "cp", "centrifugo", "#{prefix}/bin/centrifugo"
  end
end
