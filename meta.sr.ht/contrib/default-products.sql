-- Populate the database with sample products for development purposes
INSERT INTO product (id, name, subsidized, retired)
VALUES
	(1, 'Amateur Hackers', 'f', 't'),
	(2, 'Typical Hackers', 'f', 't'),
	(3, 'Professional Hackers', 'f', 't'),
	(4, 'Amateur Hackers', 'f', 'f'),
	(5, 'Typical Hackers', 'f', 'f'),
	(6, 'Professional Hackers', 'f', 'f'),
	(7, 'Subsidized service', 't', 'f');

INSERT INTO product_price (id, product_id, currency, amount)
VALUES
	(1, 1, 'USD', 200),
	(2, 2, 'USD', 500),
	(3, 3, 'USD', 1000),
	(4, 1, 'EUR', 200),
	(5, 2, 'EUR', 500),
	(6, 3, 'EUR', 1000),
	(7, 4, 'USD', 400),
	(8, 5, 'USD', 800),
	(9, 6, 'USD', 1200),
	(10, 7, 'USD', 200),
	(11, 4, 'EUR', 400),
	(12, 5, 'EUR', 800),
	(13, 6, 'EUR', 1200),
	(14, 7, 'EUR', 200);
