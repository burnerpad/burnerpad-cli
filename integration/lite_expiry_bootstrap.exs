unless System.get_env("BURNERPAD_EXPIRY_TEST") == "1" do
  raise "BURNERPAD_EXPIRY_TEST=1 is required"
end

ready_file = System.fetch_env!("BURNERPAD_EXPIRY_READY_FILE")

unless Path.type(ready_file) == :absolute do
  raise "BURNERPAD_EXPIRY_READY_FILE must be absolute"
end

# The application has already completed its normal validated boot. Override
# only this test VM's runtime lookup so the real Store expiry path can be
# exercised without patching or copying burnerpad-lite source.
Application.put_env(:burnerpad, :ttl_seconds, 5)

unless Burnerpad.Config.get(:ttl_seconds) == 5 do
  raise "could not install the expiry-test lifetime"
end

File.write!(ready_file, "ready\n", [:exclusive])
