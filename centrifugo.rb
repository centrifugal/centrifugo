class Centrifugo < Formula
  desc "Centrifugo"
  homepage "https://github.com/centrifugal/centrifugo"
  url "https://github.com/centrifugal/centrifugo/releases/download/v1.6.2/centrifugo-1.6.2-darwin-amd64.zip"
  sha256 "a1b3d6b9ad5c7a53d95e70357f9a7e25f664e37cb15473fa8ea84d5a7b0db6cf"

  def install
    system "chmod", "+x", "centrifugo"
    system "mkdir", "#{prefix}/bin"
    system "cp", "centrifugo", "#{prefix}/bin/centrifugo"
  end
end
