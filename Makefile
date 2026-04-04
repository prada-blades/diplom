install-booking:
	mkdir -p "$(HOME)/.local/bin"
	ln -sf "$(CURDIR)/booking" "$(HOME)/.local/bin/booking"
	@echo "booking installed to $(HOME)/.local/bin/booking"
