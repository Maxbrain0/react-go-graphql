import React from "react";
import { Link } from "@reach/router";
import { AuthContext } from "./contexts/AuthContext";

const Navbar: React.FC = props => {
  const authContext = React.useContext(AuthContext);
  const [navOpen, setNavOpen] = React.useState<boolean>(false);

  const toggleNav = function(): void {
    const newState = !navOpen;
    setNavOpen(newState);
  };

  const activeClass = navOpen ? "is-active" : "";

  return (
    <nav className="navbar is-fixed-top">
      <div className="navbar-brand">
        <Link to="/" className="navbar-item">
          <img
            src={`${process.env.PUBLIC_URL}/android-chrome-192x192.png`}
            alt="Home Link Logo"
          />
        </Link>
        <button
          onClick={toggleNav}
          className={`navbar-burger burger ${activeClass}`}
        >
          <span></span>
          <span></span>
          <span></span>
        </button>
      </div>
      <div className={`navbar-menu ${activeClass}`}>
        {authContext.user && (
          <div className="navbar-start">
            {authContext.user.roles.admin && (
              <Link to="/users" className="navbar-item">
                Users
              </Link>
            )}
          </div>
        )}

        <div className="navbar-end">
          {authContext.user ? (
            <div className="navbar-item">
              <button onClick={authContext.logout} className="button is-link">
                Sign Out
              </button>
            </div>
          ) : (
            <div className="navbar-item">
              <Link to="/login" className="button is-link">
                Sign In
              </Link>
            </div>
          )}
        </div>
      </div>
    </nav>
  );
};

export default Navbar;
