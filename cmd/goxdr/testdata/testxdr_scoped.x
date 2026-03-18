enum Color {
     red = 0,
     green = 1,
     blue = 2
};

union Shape switch (Color c) {
case red:
     int radius;
case green:
     int width;
default:
     void;
};

typedef bool Bool;

union U1 switch (Bool b) {
case TRUE:
     int val;
default:
     void;
};
