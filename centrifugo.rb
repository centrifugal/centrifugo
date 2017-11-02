class Centrifugo < Formula
  desc "Centrifugo"
  homepage "https://github.com/centrifugal/centrifugo"
  url "https://github.com/centrifugal/centrifugo/releases/download/v1.7.5/centrifugo-1.7.5-darwin-amd64.zip"
  sha256 "73c24e47cf0676078720445ac9e3f8ee5db0c4b8bb4ffa2bdd44a37a715498d6"

  def install
    system "chmod", "+x", "centrifugo"
    system "mkdir", "#{prefix}/bin"
    system "cp", "centrifugo", "#{prefix}/bin/centrifugo"
  end
end
