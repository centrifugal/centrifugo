class Centrifugo < Formula
  desc "Centrifugo"
  homepage "https://github.com/centrifugal/centrifugo"
  url "https://github.com/centrifugal/centrifugo/releases/download/v1.7.7/centrifugo-1.7.7-darwin-amd64.zip"
  sha256 "f8a09dc6be7fc1bf64e4445d4c93983f0484f8574159f45a0effaf38aeacef94"

  def install
    system "chmod", "+x", "centrifugo"
    system "mkdir", "#{prefix}/bin"
    system "cp", "centrifugo", "#{prefix}/bin/centrifugo"
  end
end
